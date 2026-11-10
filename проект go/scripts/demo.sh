#!/usr/bin/env bash
# Демонстрация всех лабораторных по шагам. Требует запущенную систему (make up).
set -euo pipefail

B=${API_URL:-http://localhost:28080}
LOGIN="pilot_$RANDOM"
PASS="poekhali-61"

step() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }
json_field() { python3 -c "import sys,json;print(json.load(sys.stdin)['$1'])"; }

step "ЛР1: GET /test"
curl -s "$B/test"; echo

step "ЛР2: POST /dbtest (строка из тела запроса пишется в БД)"
curl -s -X POST "$B/dbtest" -d "Поехали! $(date +%T)"
curl -s "$B/dbtest" | head -c 300; echo

step "ЛР3: регистрация пользователя $LOGIN"
curl -s -X POST "$B/auth/register" -H 'Content-Type: application/json' \
  -d "{\"login\":\"$LOGIN\",\"password\":\"$PASS\"}"

step "ЛР3: авторизация и получение JWT"
TOKEN=$(curl -s -X POST "$B/auth/login" -d "{\"login\":\"$LOGIN\",\"password\":\"$PASS\"}" | json_field token)
echo "token = ${TOKEN:0:40}..."

step "Каталог направлений"
curl -s "$B/destinations"

step "ЛР4: создание заявок на полёт"
curl -s -X POST "$B/orders" -H "Authorization: Bearer $TOKEN" -d '{"destination":"moon","passengers":2}'
curl -s -X POST "$B/orders" -H "Authorization: Bearer $TOKEN" -d '{"destination":"mars","passengers":1}'

step "ЛР5: без токена — 401"
curl -s -w ' [HTTP %{http_code}]\n' "$B/orders"

step "ЛР4: статусы заявок сразу после создания"
curl -s "$B/orders" -H "Authorization: Bearer $TOKEN"; echo

step "Ждём 12 секунд, пока ЦУП (mission-control) обработает заявки через RabbitMQ..."
sleep 12
curl -s "$B/orders" -H "Authorization: Bearer $TOKEN"; echo
