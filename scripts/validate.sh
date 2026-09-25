#!/usr/bin/env sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
set -a
if [ -f .env ]; then . ./.env; else . ./.env.example; fi
set +a

(command -v jq >/dev/null 2>&1) || { echo "jq is required for API validation" >&2; exit 1; }

cleanup() { docker compose down -v --remove-orphans; }
docker compose down -v --remove-orphans
if [ "${KEEP_RUNNING:-0}" = "1" ]; then
	trap cleanup INT TERM
else
	trap cleanup EXIT INT TERM
fi

(cd backend && go test ./... && go vet ./... && go build ./...)
(cd frontend && npm ci --no-audit --no-fund && npm run typecheck && npm run build)
docker compose config --quiet
docker compose up -d --build

i=0
until curl -fsS "http://127.0.0.1:${BACKEND_PORT:-19520}/healthz" >/dev/null; do
	i=$((i+1))
	[ "$i" -lt 60 ] || { docker compose logs; exit 1; }
	sleep 2
done
curl -fsS "http://127.0.0.1:${FRONTEND_PORT:-18520}/" >/dev/null

login_token() {
	curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT:-19520}/api/auth/login" \
		-H 'Content-Type: application/json' \
		-d "{\"username\":\"$1\",\"password\":\"Admin123!\"}" | jq -er '.data.token'
}

viewer_token=$(login_token viewer)
operator_token=$(login_token operator)
reviewer_token=$(login_token reviewer)
admin_token=$(login_token admin)

curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/session" -H "Authorization: Bearer $viewer_token" | jq -e '.data.role == "viewer"' >/dev/null
viewer_write_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/bridges" -H "Authorization: Bearer $viewer_token" -H 'Content-Type: application/json' -d '{}')
[ "$viewer_write_status" = "403" ]
viewer_audit_status=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${BACKEND_PORT}/api/audits" -H "Authorization: Bearer $viewer_token")
[ "$viewer_audit_status" = "403" ]
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audits?page=1&pageSize=20" -H "Authorization: Bearer $reviewer_token" | jq -e '.data | type == "array"' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/runtime" -H "Authorization: Bearer $admin_token" | jq -e '.data.appName and .data.databaseDriver and (.data.requestLimit > 0)' >/dev/null

now=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
code="PD-SMOKE-$(date +%s)"
create_payload=$(jq -n --arg code "$code" --arg now "$now" '{code:$code,name:"空卷验收优先级决定",description:"验证不可变版本链",facility:"K42 桥梁作业区",owner:"现场处置组",category:"结构缺陷",riskLevel:"critical",metricValue:88,metricUnit:"score",effectiveAt:$now,evidence:"裂缝照片与量测记录 v1",relatedCode:"DF-001"}')
created=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities" -H "Authorization: Bearer $operator_token" -H 'X-Request-ID: smoke-create' -H 'Content-Type: application/json' -d "$create_payload")
priority_id=$(printf '%s' "$created" | jq -er '.data.id')
printf '%s' "$created" | jq -e '.data.status == "draft" and .data.version == 1 and .data.preparedBy == "operator" and (.data.revisions | length == 1)' >/dev/null

update_payload=$(jq -n --arg now "$now" '{expectedVersion:1,name:"空卷验收优先级决定",description:"复核前补充量测证据",facility:"K42 桥梁作业区",owner:"现场处置组",category:"结构缺陷",riskLevel:"critical",metricValue:93,metricUnit:"score",effectiveAt:$now,evidence:"裂缝照片、量测记录与复测记录 v2",relatedCode:"DF-001"}')
updated=$(curl -fsS -X PUT "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id" -H "Authorization: Bearer $operator_token" -H 'X-Request-ID: smoke-update' -H 'Content-Type: application/json' -d "$update_payload")
printf '%s' "$updated" | jq -e '.data.version == 2 and (.data.revisions | length == 2)' >/dev/null

transition_payload='{"status":"urgent","expectedVersion":2,"reason":"独立复核确认需立即处置"}'
operator_final_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$transition_payload")
[ "$operator_final_status" = "403" ]

self_code="PD-SELF-$(date +%s)"
self_payload=$(printf '%s' "$create_payload" | jq --arg code "$self_code" '.code = $code')
self_created=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "$self_payload")
self_id=$(printf '%s' "$self_created" | jq -er '.data.id')
self_final_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$self_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d '{"status":"observe","expectedVersion":1,"reason":"不得自行复核自己的决定"}')
[ "$self_final_status" = "422" ]

curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'X-Request-ID: smoke-review' -H 'Content-Type: application/json' -d "$transition_payload" | jq -e '.data.status == "urgent" and .data.version == 3' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id" -H "Authorization: Bearer $reviewer_token" | jq -e '
	.data.status == "urgent" and
	(.data.revisions | length == 3) and
	([.data.revisions[].evidence] == ["裂缝照片与量测记录 v1","裂缝照片、量测记录与复测记录 v2","裂缝照片、量测记录与复测记录 v2"]) and
	([.data.revisions[].actor] == ["operator","operator","reviewer"]) and
	([.data.revisions[].requestId] == ["smoke-create","smoke-update","smoke-review"])' >/dev/null

locked_status=$(curl -sS -o /dev/null -w '%{http_code}' -X PUT "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$priority_id" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$(printf '%s' "$update_payload" | jq '.expectedVersion = 3')")
[ "$locked_status" = "422" ]
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audit-summary?windowHours=24" -H "Authorization: Bearer $reviewer_token" | jq -e '.data.total >= 3 and .data.transitions >= 1' >/dev/null

# --- 限速定稿联动桥梁受限 / 关闭桥梁拒绝定稿 / 受限恢复守卫 ---
k42_bridge_id=$(curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/bridges?search=BA-004" -H "Authorization: Bearer $viewer_token" | jq -er '.data[0].id')
k42_bridge_version=$(curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/bridges/$k42_bridge_id" -H "Authorization: Bearer $viewer_token" | jq -er '.data.version')
# urgent 定稿后同设施 BA-004 必须跟随进入 restricted，且版本 +1。
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/bridges/$k42_bridge_id" -H "Authorization: Bearer $viewer_token" | jq -e --argjson v "$k42_bridge_version" '.data.status == "restricted" and .data.version == ($v + 1) and (.data.unconfirmedDefectCount | type == "number")' >/dev/null

# 桥梁已关闭的设施（区域5 BA-005）定稿 restrict 必须 422 拒绝。
closed_code="PD-CLOSED-$(date +%s)"
closed_payload=$(jq -n --arg code "$closed_code" --arg now "$now" '{code:$code,name:"关闭桥梁设施限速决定",facility:"铁路桥梁封闭停用区域5",owner:"安全主管组",category:"封闭",riskLevel:"high",metricValue:60,metricUnit:"score",effectiveAt:$now,evidence:"关闭桥梁拒绝定稿验证",relatedCode:"BA-005"}')
closed_created=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$closed_payload")
closed_id=$(printf '%s' "$closed_created" | jq -er '.data.id')
closed_final_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/priorities/$closed_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d '{"status":"restrict","expectedVersion":1,"reason":"关闭桥梁不应放行该定稿"}')
[ "$closed_final_status" = "422" ]

# K42 桥梁已受限：复核人在仍有确认阶段缺陷时恢复 active 被拒绝，错误写明剩余条数。
# 先登记一条 K42 新缺陷。
defect_code="DF-K42-$(date +%s)"
defect_payload=$(jq -n --arg code "$defect_code" --arg now "$now" '{code:$code,name:"K42限速联动缺陷",facility:"K42 桥梁作业区",owner:"现场处置组",category:"结构",riskLevel:"high",metricValue:55,metricUnit:"score",effectiveAt:$now,evidence:"裂缝照片与量测记录",relatedCode:"BA-004"}')
defect_id=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/defects" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$defect_payload" | jq -er '.data.id')

bridge_denied=$(curl -sS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/bridges/$k42_bridge_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d '{"status":"active","expectedVersion":2,"reason":"缺陷尚未离开确认阶段应被拒绝"}')
printf '%s' "$bridge_denied" | jq -e '.error == "unconfirmed_defects" and (.message | contains("1 defect"))' >/dev/null
# operator 恢复受限桥梁必须 403。
bridge_op_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/bridges/$k42_bridge_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d '{"status":"active","expectedVersion":2,"reason":"operator无权恢复"}')
[ "$bridge_op_status" = "403" ]
# 缺陷推进 new -> monitoring 离开确认阶段后，复核人成功恢复 active。
curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/defects/$defect_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d '{"status":"monitoring","expectedVersion":1,"reason":"缺陷进入持续监测阶段"}' | jq -e '.data.status == "monitoring"' >/dev/null
curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/bridges/$k42_bridge_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d '{"status":"active","expectedVersion":2,"reason":"确认阶段缺陷清零复核恢复运行"}' | jq -e '.data.status == "active" and .data.version == 3 and .data.unconfirmedDefectCount == 0' >/dev/null

docker compose ps
if [ "${KEEP_RUNNING:-0}" = "1" ]; then
	echo "KEEP_RUNNING=1: containers left running for browser validation"
else
	cleanup
	trap - EXIT INT TERM
fi
