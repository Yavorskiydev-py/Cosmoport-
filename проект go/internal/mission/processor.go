// Package mission — сервис обработки заявок "Центр управления полётами" (ЦУП).
//
// Он получает из очереди orders.created новые заявки, имитирует подготовку и проведение
// полёта (с задержками) и публикует каждое изменение статуса в очередь orders.status.
package mission

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	amqp "github.com/rabbitmq/amqp091-go"

	"cosmoport/internal/contracts"
)

var (
	missionsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "mission_control", Name: "missions_received_total",
		Help: "Orders received from the api service.",
	})
	missionsInProgress = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "mission_control", Name: "missions_in_progress",
		Help: "Missions being processed right now.",
	})
	statusTransitions = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "mission_control", Name: "status_transitions_total",
		Help: "Status changes published to the api service.",
	}, []string{"status"})
	missionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "mission_control", Name: "mission_duration_seconds",
		Help:    "Total processing time of a mission.",
		Buckets: []float64{1, 2, 4, 6, 8, 10, 15, 20, 30, 60},
	}, []string{"result"})
	missionsRequeued = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "mission_control", Name: "missions_requeued_total",
		Help: "Missions returned to the queue (shutdown or publish error).",
	})
)

// Publisher публикует сообщения в брокер.
type Publisher interface {
	Publish(ctx context.Context, queue string, v any) error
}

// Consumer отдаёт поток сообщений из очереди.
type Consumer interface {
	Consume(queue, consumerTag string, prefetch int) (<-chan amqp.Delivery, *amqp.Channel, error)
}

// Config — настройки имитации.
type Config struct {
	Workers       int           // сколько заявок обрабатывается параллельно
	MinStageDelay time.Duration // минимальная длительность одного этапа
	MaxStageDelay time.Duration // максимальная длительность одного этапа
	RejectRate    float64       // доля отклонённых заявок (0..1)
}

// stage — один этап жизненного цикла заявки.
type stage struct {
	status string
	reason string
}

var rejectReasons = []string{
	"Солнечная вспышка: старт отменён",
	"Экипаж не прошёл медкомиссию",
	"Неисправность двигателя второй ступени",
	"Метеоритный поток на траектории",
	"Нет свободных мест на борту",
}

// Processor — обработчик заявок.
type Processor struct {
	pub  Publisher
	cons Consumer
	cfg  Config
	log  *slog.Logger
}

// NewProcessor создаёт обработчик.
func NewProcessor(pub Publisher, cons Consumer, cfg Config, log *slog.Logger) *Processor {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.MaxStageDelay < cfg.MinStageDelay {
		cfg.MaxStageDelay = cfg.MinStageDelay
	}
	return &Processor{pub: pub, cons: cons, cfg: cfg, log: log}
}

// Run запускает пул воркеров и блокируется до отмены ctx.
// При остановке незавершённые заявки возвращаются в очередь (Nack с requeue)
// и будут обработаны после перезапуска — сообщения не теряются.
func (p *Processor) Run(ctx context.Context) error {
	deliveries, ch, err := p.cons.Consume(contracts.QueueOrderCreated, "mission-control", p.cfg.Workers)
	if err != nil {
		return err
	}
	defer ch.Close()

	p.log.Info("mission control started", "workers", p.cfg.Workers, "queue", contracts.QueueOrderCreated)

	var wg sync.WaitGroup
	errCh := make(chan error, p.cfg.Workers)
	for i := 0; i < p.cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case d, ok := <-deliveries:
					if !ok {
						errCh <- errors.New("deliveries channel closed")
						return
					}
					p.handle(ctx, d)
				}
			}
		}()
	}

	select {
	case <-ctx.Done():
	case err = <-errCh:
	}
	wg.Wait()
	p.log.Info("all workers stopped")
	return err
}

func (p *Processor) handle(ctx context.Context, d amqp.Delivery) {
	var order contracts.OrderCreated
	if err := json.Unmarshal(d.Body, &order); err != nil {
		p.log.Error("drop malformed order message", "error", err)
		_ = d.Nack(false, false)
		return
	}

	missionsReceived.Inc()
	missionsInProgress.Inc()
	defer missionsInProgress.Dec()
	start := time.Now()
	log := p.log.With("order_id", order.OrderID, "destination", order.Destination)
	log.Info("mission received", "passengers", order.Passengers, "redelivered", d.Redelivered)

	stages := []stage{{"processing", "ЦУП проверяет экипаж и готовит ракету"}}
	if rand.Float64() < p.cfg.RejectRate {
		stages = append(stages, stage{"rejected", rejectReasons[rand.IntN(len(rejectReasons))]})
	} else {
		stages = append(stages,
			stage{"approved", "Полёт одобрен, экипаж на борту"},
			stage{"launched", "Ключ на старт! Ракета покинула стартовый стол"},
			stage{"completed", "Экипаж успешно прибыл в пункт назначения"},
		)
	}

	for i, st := range stages {
		if i > 0 && !p.sleep(ctx) {
			log.Warn("shutdown during mission, returning to queue", "next_status", st.status)
			missionsRequeued.Inc()
			_ = d.Nack(false, true)
			return
		}
		event := contracts.OrderStatusChanged{
			OrderID: order.OrderID, Status: st.status, Reason: st.reason, ChangedAt: time.Now(),
		}
		pubCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := p.pub.Publish(pubCtx, contracts.QueueOrderStatus, event)
		cancel()
		if err != nil {
			log.Error("publish status failed, returning to queue", "status", st.status, "error", err)
			missionsRequeued.Inc()
			_ = d.Nack(false, true)
			return
		}
		statusTransitions.WithLabelValues(st.status).Inc()
		log.Info("status published", "status", st.status, "reason", st.reason)
	}

	result := stages[len(stages)-1].status
	missionDuration.WithLabelValues(result).Observe(time.Since(start).Seconds())
	_ = d.Ack(false)
	log.Info("mission finished", "result", result, "duration_ms", time.Since(start).Milliseconds())
}

// sleep ждёт случайное время этапа. Возвращает false, если сервис останавливается.
func (p *Processor) sleep(ctx context.Context) bool {
	delay := p.cfg.MinStageDelay
	if spread := p.cfg.MaxStageDelay - p.cfg.MinStageDelay; spread > 0 {
		delay += rand.N(spread)
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
