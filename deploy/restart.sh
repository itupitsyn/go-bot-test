#!/bin/bash
#
# Перезапуск бота без потерь. Запускается деплоем вместо голого
# `systemctl restart raffle-bot`.
#
# ЗАЧЕМ. Перезапуск сам по себе уже не рубит начатое: бот перестаёт брать новое
# и дожидается обработчиков. Но ждёт он ограниченное время, а задача живёт
# дольше, чем кажется — вместе с очередью. Замеры за сутки 14.10.2026:
#
#     i2v     90 задач   ожидание 161.8 с + счёт 113.7 с = 275.5 с в среднем
#     t2v      5 задач   ожидание 175.6 с + счёт 178.0 с = 353.6 с в среднем
#     максимальное ожидание в очереди              1024.6 с
#
# При отсрочке 300 секунд средний t2v не проходит вовсе, а половина i2v — как
# повезёт. Профилактика эту разницу и закрывает: новые задачи не встают в
# очередь, пока мы ждём, и ждать приходится только уже начатое.
#
# ЧЕГО ЭТОТ СКРИПТ НЕ ДЕЛАЕТ. Он не ждёт опустошения очереди перед
# перезапуском: ждать умеет сам бот, и делать это дважды незачем. Всё, что он
# всё-таки не успеет отдать, подбирает журнал незабранного уже после старта.
# Два механизма дополняют друг друга, а не дублируют.
set -u

APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE=raffle-bot

cd "$APP_DIR" || { echo "нет каталога $APP_DIR"; exit 1; }

if [ ! -f .env.local ]; then
	echo "нет .env.local в $APP_DIR — без него не узнать базу"
	exit 1
fi

set -a
# shellcheck disable=SC1091
. ./.env.local
set +a

export PGPASSWORD="${DB_PASSWORD:-}"
PSQL=(psql -h "${DB_HOST:-localhost}" -p "${DB_PORT:-5432}" -U "${DB_USER:-}" -d "${DB_NAME:-}" -At -q -v ON_ERROR_STOP=1)

sql() { "${PSQL[@]}" -c "$1"; }

# Состояние профилактики ДО нас, чтобы вернуть ровно как было. Трогаем два
# поля: is_enabled и ends_at.
#
# ends_at важен не меньше флага. IsActive считает профилактику законченной,
# когда срок прошёл, поэтому строка с is_enabled = true и прошедшим ends_at
# ничего не блокирует. Включи мы только флаг — профилактика осталась бы
# декорацией, а деплой срезал бы людей ровно как раньше.
WAS_ROW=$(sql "select is_enabled || '|' || coalesce(to_char(ends_at, 'YYYY-MM-DD HH24:MI:SSOF'), '') from ai_maintenances where id = 1")
PSQL_RC=$?

if [ $PSQL_RC -ne 0 ]; then
	echo "база недоступна, профилактику не трогаю — перезапускаю как есть"
	systemctl restart "$SERVICE"
	systemctl is-active "$SERVICE"
	exit $?
fi

WE_ENABLED_IT=no

restore() {
	[ "$WE_ENABLED_IT" = yes ] || return 0

	local was_enabled was_ends ends_sql rc

	if [ -z "$WAS_ROW" ]; then
		# Строки не было вовсе: админка её ни разу не сохраняла. Возвращаем то
		# же по смыслу состояние — выключено и без срока.
		was_enabled=false
		ends_sql=null
	else
		was_enabled="${WAS_ROW%%|*}"
		was_ends="${WAS_ROW#*|}"
		ends_sql=null
		[ -n "$was_ends" ] && ends_sql="'$was_ends'"
	fi

	sql "update ai_maintenances set is_enabled = $was_enabled, ends_at = $ends_sql, updated_at = now() where id = 1" >/dev/null
	rc=$?

	if [ $rc -eq 0 ]; then
		echo "профилактика возвращена в прежнее состояние (is_enabled = $was_enabled)"
	else
		echo "ВНИМАНИЕ: не смог вернуть профилактику, выключи её руками в админке"
	fi
}

# Через trap, а не в конце: если скрипт упадёт посреди перезапуска, бот не
# должен остаться молчащим для людей. Лучше неудачный деплой, чем тихий бот.
trap restore EXIT

ALREADY_ACTIVE=$(sql "select coalesce((select is_enabled and (ends_at is null or ends_at > now()) from ai_maintenances where id = 1), false)")

if [ "$ALREADY_ACTIVE" = t ]; then
	echo "профилактика уже включена — не трогаю её и не буду выключать"
else
	sql "insert into ai_maintenances (id, is_enabled, ends_at, updated_at)
	     values (1, true, null, now())
	     on conflict (id) do update set is_enabled = true, ends_at = null, updated_at = now()" >/dev/null
	if [ $? -ne 0 ]; then
		echo "не смог включить профилактику — перезапускаю без неё"
	else
		WE_ENABLED_IT=yes
		echo "профилактика включена, новые задачи в очередь не встают"
	fi
fi

echo "перезапуск $SERVICE, ждём начатое..."
STARTED=$(date +%s)
systemctl restart "$SERVICE"
RC=$?
echo "перезапуск занял $(( $(date +%s) - STARTED )) с"

if [ $RC -ne 0 ]; then
	echo "systemctl restart вернул $RC"
	exit $RC
fi

# Короткая проверка, что бот не падает на старте: без неё деплой считался бы
# удачным, даже если процесс поднялся и сразу умер на миграциях.
sleep 3
if ! systemctl is-active --quiet "$SERVICE"; then
	echo "служба не активна через 3 секунды после старта:"
	systemctl status "$SERVICE" --no-pager -l | tail -20
	exit 1
fi

echo "служба активна, PID $(systemctl show "$SERVICE" -p MainPID --value)"
