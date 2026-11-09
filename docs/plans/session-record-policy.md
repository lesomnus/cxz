# 항목별 세션 기록 정책 — 분류·보존·가용성 계약

상위 구현계획: cxz#335 · 이 문서는 단계 1(cxz#336)의 산출물이다. 후속 단계(Recorder/Store/API/UI)는 이 계약을 기준으로 구현한다.

## 1. 왜 분류가 필요한가

세션 기록은 지금 하나의 예산을 쓴다. 원본 프로토콜, 대화, 도구 인자와 출력, 모델 목록, 사용량 상태가 모두 `events`에 들어가고, 기본 100MiB를 넘으면 가장 오래된 완료 턴부터 지워 약 80%로 줄인다.

2026-11-09에 이 설치의 세션 5개를 측정했다(각 `conversation.db`를 `VACUUM INTO`로 스냅샷한 뒤 `sessionstore.Store`로 이벤트 단위 집계). 결과는 그 예산이 무엇에 쓰이는지에 대한 것이다.

| 세션 | 이벤트 바이트 | 보존 이벤트 / 전체 seq | 본문 텍스트 |
|---|---|---|---|
| A (claude, working) | 94.6MiB | 49,661 / 431,455 (11.5%) | 4KiB |
| B (codex, idle) | 91.1MiB | 129,219 / 859,299 (15.0%) | 15KiB |
| C (codex, working) | 95.2MiB | 119,568 / 1,464,761 (8.2%) | 13KiB |
| D (정지) | 95.3MiB | 125,805 / 125,805 (100%) | 6KiB |
| E (codex, idle) | 86.0MiB | 117,434 / 474,242 (24.8%) | 12KiB |

정리는 동작한다. 그래서 문제가 된다. 모든 세션이 100MiB 천장에 붙어 trim과 재증식을 반복하고, 이력의 75~92%가 이미 삭제됐다. 그 예산을 차지한 것은 대화가 아니다 — 본문 텍스트는 4~15KiB다.

예산을 차지한 것들에는 **네 가지 서로 다른 원인**이 있고, 하나의 정책으로는 넷 중 하나만 고칠 수 있다. 이 문서의 분류표는 네 원인에 각각 답하는 것을 목표로 한다.

### 원인 1 — 현재 값의 반복 (해결: cxz#344, cxz#345)

`models:catalog`는 5개 세션 합계 33,416건 46.2MiB에 서로 다른 값이 13개였다. codex `usage:account/rateLimits/updated`는 25,012건 32.9MiB에 969개였다. 분마다 같은 값을 다시 기록한 결과다.

판정 기준은 payload 동일성이 아니다. claude `usage:get_usage`는 12,301건 중 12,293개가 서로 다른 payload였고, 원인은 append 직전에 주입되는 `_cxz_observed_ms`였다. 비교는 관측 시각 같은 비의미 필드를 제외한 **의미 투영**으로 해야 한다.

이 원인은 단계 0에서 처리됐다. 측정된 분포 기준 79.1MiB → 4.9MiB.

### 원인 2 — 같은 바이트의 다중 저장

codex 한 세션(B)의 도구 기록:

| 내용 | 저장되는 곳 | 합계 | 서로 다른 바이트 |
|---|---|---|---|
| `command` + `commandActions` | `approval.params`, `tool_call.item`, `tool_result.item` | 4.62MiB | 1.54MiB |
| 명령 출력 | `tool_result`의 `e.Text`, 같은 이벤트의 `payload.item.aggregatedOutput` | 12.19MiB | 약 6.3MiB |

출력 중복의 출처는 한 줄이다: `s.event("tool_result", p.Item.Output, p.Item.ID, v.Params, nil)` — `e.Text`가 출력이고 `v.Params`가 그 출력을 다시 담는다. 세션 C는 4.53/1.51MiB와 12.55MiB, E는 4.77/1.59MiB와 7.28MiB로 같은 비율이다.

`intent`와 `receipt`도 같은 `command`를 각각 저장한다(세션당 0.29+0.29MiB).

### 원인 3 — 정규화되지 않은 provider payload

claude `usage:get_usage` 34.67MiB의 구성은 `rate_limits` **29.20MiB(84.2%)**, `session` 3.81MiB(11%)다. 유일하게 렌더링되는 필드가 payload의 84%이고, 이벤트당 그 필드만 2.48KiB다.

그런데 그 안에서 화면에 닿는 정보는 window 몇 개의 잔여 퍼센트와 리셋 시각이다. `agentview.Quota`가 이미 그 투영을 만든다. 즉 이것은 반복도 미사용 필드도 아니라 **무엇을 싣는가**의 문제다.

### 원인 4 — 응답 사본과 반복되는 provider 상수

남아 있는 raw payload의 구성:

| 세션 | raw | 최대 항목 |
|---|---|---|
| A (claude) | 17.30MiB | `control_response` 13.75MiB (79.5%), n=5,947 |
| B (codex) | 18.66MiB | id/result 형태 11.50MiB (61.7%), n=83,451 |
| D (codex) | 42.32MiB | id/result 형태 42.17MiB (99.6%), n=93,486 |
| E (codex) | 19.63MiB | id/result 형태 16.25MiB (82.7%), n=81,742 |

claude의 `control_response` 13.75MiB는 `get_usage`·`list_models` 응답이다. **같은 poll이 두 번 기록된다**: 정규화된 이벤트로 한 번, 응답의 verbatim 사본으로 또 한 번. raw를 하나의 범주로 두면 대화의 원본 스트림과 폴링 응답의 사본이 같은 예산과 같은 보존 형태를 갖는다.

승인에는 호출마다 반복되는 provider 상수가 있다: `approval.params.availableDecisions` 0.67~0.82MiB(선택지 목록), `params.proposedExecpolicyAmendment` 0.52~0.64MiB.

### 그밖에 계약이 설명해야 하는 것

- **빈 raw 껍데기 271,912건.** `Shed`는 payload를 비우고 자리를 남기며, 그 빈 body 자체가 "턴이 raw를 만들었다"는 의미를 담는다. 상위 계획의 "빈 body로 의미를 추측하지 않는다"와 충돌하므로 소급 해석이 필요하다.
- **base64 1.33배.** raw는 JSON 안에 base64로 저장된다. 남아 있던 payload 115.2MiB가 약 153MiB로 계상됐다. 한도는 부풀려진 쪽에서 측정된다.
- **단일 거대 최신값.** `history_checkpoint` 한 건이 1.32 / 3.28 / 5.12 / 10.81MiB다. `makeHistoryCheckpoint`가 의미 키별 최신값을 한 블롭에 모으고 trim마다 재직렬화한다.
- **정리 시점의 단일 게이트.** 본문 compact, raw 축소, 최신값 병합이 모두 `internal/supervisor/history.go:115`의 한 조건 뒤에 있다. idle이 아니거나 활성 background 작업이 있으면 턴 경계가 필요 없는 정리까지 함께 지연된다.

## 2. 설정 축

세 축은 독립이고, 범주마다 따로 설정된다.

| 축 | 값 | 의미 |
|---|---|---|
| `storage` | `disk` \| `memory` \| `off` | 어디에 둘지. `memory`는 Supervisor 소유이며 Supervisor 재시작에 소멸한다. `off`는 기록하지 않는다. |
| `keep` | `history` \| `latest` | 모든 발생을 남길지, 의미 키별 최신값만 남길지. |
| 한도 | `bytes`, `events`, `keys` | 범주별 상한. 미지정은 기본값, 명시적 0은 무제한. |

`keep: latest`는 키 단위로 셈한다(`keys`), `keep: history`는 바이트와 건수로 셈한다(`bytes`, `events`). 한도는 이벤트 바이트로 측정한다 — 파일 크기나 SQLite 페이지가 아니다(cxz#286).

필수 복구 정보는 `storage`를 받지 않는다. `memory`나 `off`를 요청하면 거부한다.

## 3. 분류표

`kind`만으로 분류하지 않는다. 한 `kind`가 여러 의미를 담으면 `text`(subtype)나 payload 필드까지 본다.

| 범주 | 원천 | storage | keep | 기본 한도 | 비고 |
|---|---|---|---|---|---|
| `conversation` | `input`, `assistant`, `tool_output`의 text, `compact` | disk | history | 64MiB | **다른 범주의 초과로 삭제되지 않는다.** 측정상 4~15KiB. |
| `turn_metrics` | `turn_end`, `usage:thread/tokenUsage/updated`, `usage:context/message`, `response` 스냅샷 | disk | history | 8MiB | 턴이 쓴 값은 그 턴의 사실이다. 현재 상태로 덮어쓰지 않는다. |
| `work_summary` | `tool_call`의 이름·대상·상태, `tool_result`의 성공/실패, `approval`의 결정 | disk | history | 16MiB | 상세가 만료돼도 작업의 존재·이름·상태·파일 요약이 남는 최소 집합. |
| `work_detail` | `tool_call.item.arguments`, `tool_result.item.result`, `item.aggregatedOutput` | disk | history | 32MiB | 측정상 세션당 20~27MiB. 범주 초과 시 본문보다 먼저 만료된다. |
| `work_diff` | `turn/diff/updated`, `item.changes` | disk | history | 8MiB | |
| `account_limits` | `usage:get_usage`, `usage:account/rateLimits/updated`, `usage:rate_limit_event` | disk | latest | 64 keys | 키는 `account/provider/window`. weekly와 5-hour가 공존한다. |
| `context_state` | 컨텍스트 잔여량 | disk | latest | 16 keys | 키는 `session/run`. |
| `model_catalog` | `models:catalog` | disk | latest | 8 keys | 측정상 33,416건에 값 13개. |
| `availability` | `usage_status`, `models_status`, `setting_status` | disk | latest | 32 keys | 키는 `kind/subject`. |
| `provider_constant` | `approval.params.availableDecisions`, `params.proposedExecpolicyAmendment` | disk | latest | 16 keys | 승인마다 반복되는 provider 상수. 키는 `provider/method`. 측정상 세션당 1.2~1.5MiB. |
| `provider_stream` | 대화를 만든 raw(assistant/user/item 델타) | disk | history | 16MiB | raw 중 대화의 원본. |
| `provider_reply` | 정규화된 상태를 만든 raw 응답(`control_response`, id/result) | off | history | 8MiB | 측정상 raw의 80~99%. 기본적으로 기록하지 않는다. 한도는 켤 때를 위한 것이다 — 예산 없는 범주를 off 스위치 뒤에 두지 않는다. §5. |
| `diagnostic` | `diagnostic`, `error`, `update`, `history_shed` | disk | history | 4MiB | |
| `control` | `state`, `intent`, `receipt`, `permission`, `approval`의 미해결 상태, `setting`, `queued`, `background`, run/vendor identity, sequence high-water | disk | latest | 4096 keys | **`storage`를 받지 않는다.** §6. |

### 한 kind를 쪼개는 지점

| kind | 쪼개는 기준 | 가는 곳 |
|---|---|---|
| `usage` | `text` | `get_usage`·`account/rateLimits/updated`·`rate_limit_event` → `account_limits`; `thread/tokenUsage/updated`·`context/message` → `turn_metrics` |
| `tool_call` | payload 필드 | `item.type`·`item.id`·`item.command`(무엇을 실행했는지) → `work_summary`; `item.arguments`와 그밖의 본문 → `work_detail`; `item.changes` → `work_diff` |
| `tool_result` | payload 필드 | 성공/실패·`tool_use_id` → `work_summary`; `item.result`·`item.aggregatedOutput` → `work_detail` |
| `approval` | payload 필드 | 결정과 대상 → `work_summary`; 미해결 상태 → `control`; `params.availableDecisions`·`params.proposedExecpolicyAmendment` → `provider_constant` |
| `raw` | vendor payload의 `type`/`method` 유무와 형태 | 대화 델타 → `provider_stream`; `control_response`와 id/result → `provider_reply` |

`unknown`은 안전하게 처리한다: 분류되지 않은 `kind`는 `diagnostic`의 한도를 쓰고 `keep: history`로 남으며, 삭제보다 보존을 택한다. 새 provider 이벤트가 조용히 사라지는 것이 조용히 예산을 먹는 것보다 나쁘다.

## 4. 같은 내용을 두 번 쓰지 않는다

| 현재 | 계약 |
|---|---|
| `tool_result`의 `e.Text`와 `payload.item.aggregatedOutput` | 출력의 정본은 하나다. `e.Text`를 정본으로 두고 payload에서는 제거한다. |
| `command`/`commandActions`가 `approval`·`tool_call`·`tool_result`에 각각 | 호출의 정본은 `tool_call`이다. `approval`과 `tool_result`는 `tool_use_id`로 참조한다. |
| `intent`와 `receipt`의 `command` | `receipt`는 `intent`를 참조한다. 중복 실행 방지에 필요한 것은 `client_id`와 digest다. |
| `commandActions`가 `command`의 파생 | 파생값은 기록하지 않는다. 읽는 쪽에서 만든다. |

참조가 가리키는 대상이 만료될 수 있으므로, 참조는 가용성 상태를 함께 답한다(§7).

## 5. 정규화해서 담는 범주

`account_limits`는 provider payload를 그대로 담지 않는다. `agentview.Quota`가 만드는 window 집합(키, 라벨, 잔여 퍼센트, 리셋 시각, 상태)을 담는다.

- 보존: 렌더링되는 모든 값과 관측 시각.
- 손실: `session`, `behaviors`, `subscription_type`, `rateLimitResetCredits`처럼 화면에 닿지 않는 필드.
- 원본이 필요할 때: `provider_reply`를 `disk`로 설정한 세션에서만 verbatim 응답을 얻는다. 기본값은 `off`이므로, 기본 설정에서 원본은 남지 않는다. 이것이 의도된 거래다 — 측정상 그 원본은 raw 예산의 80~99%였다.

`model_catalog`도 같은 방식으로 정규화된 모델 목록을 담는다.

## 6. 필수 복구 정보

다음은 범주 예산과 무관하게 보호되며 `memory`/`off`를 받지 않는다. 요청하면 거부한다.

- 재시작 복구: 세션 상태, run identity, vendor/thread identity, sequence high-water와 generation.
- 미해결 제어: 승인, 질문, 대기 입력.
- 중복 실행 방지: 명령 receipt(`client_id` + digest + status).
- background 작업의 마지막 정규화 스냅샷.
- 설정의 현재 값.

현재 이 역할은 `makeHistoryCheckpoint`가 trim 시점에 `setting:*`, `receipt:*`, `background:*` 키로 수행한다. 새 계약은 그 집합을 출발점으로 삼고 **키별 레코드**로 표현한다. 단일 블롭(측정 최대 10.81MiB)을 만들지 않고, 같은 정보를 두 곳에서 관리하지 않는다.

receipt의 보장 범위는 명시한다: 가장 오래된 보호 receipt보다 앞선 명령의 중복 여부는 답하지 않는다.

## 7. 가용성

body는 네 상태를 갖는다. 이름은 여기서 확정한다.

| 상태 | 의미 |
|---|---|
| `available` | 기록돼 있고 읽을 수 있다. |
| `expired` | 기록됐으나 보존 한도로 제거됐다. 원래 크기와 범주, 제거 시점을 함께 답한다. |
| `not_recorded` | 정책이 `off`였으므로 기록되지 않았다. 범주와 그때 적용된 정책을 답한다. |
| `volatile_lost` | `memory`였고 Supervisor가 재시작되어 소멸했다. |

규칙:

- 빈 body나 sequence hole만으로 의미를 추측하지 않는다. 상태는 명시적으로 답한다.
- 상세가 없는 것을 도구 실행이 없었던 것으로 취급하지 않는다.
- 대화 전체의 prefix floor와 개별 body의 만료는 다른 개념이다. 전자는 커서가 되고 후자는 body의 상태가 된다.

### 기존 빈 raw 껍데기의 소급 해석

전환 시점에 이미 존재하는 `kind="raw"` + 빈 payload 이벤트 271,912건은 `Shed`가 만든 것과 원래 비어 있던 것을 구분할 수 없다. 규칙:

- `history_shed` 경계 이벤트가 가리키는 `Through` 이하의 빈 raw는 `expired`로 읽는다.
- 그 경계가 없는 빈 raw는 `expired`로 읽되 원래 크기를 `unknown`으로 답한다. `available`로 읽어 빈 내용을 보여주지 않는다.
- 새 기록은 자리를 남기지 않는다. 번호와 coverage는 작은 별도 상태로 유지한다.

## 8. 정리 시점

범주마다 정리가 허용되는 시점이 다르다.

| 범주 | 완료 턴 경계 필요 | 이유 |
|---|---|---|
| `conversation` | 필요 | 재생 가능한 지점에서만 prefix를 잘라낼 수 있다. |
| `work_detail`, `work_diff`, `provider_stream` | 불필요 | 개별 body의 만료는 재생 지점과 무관하다. 단 진행 중 턴과 그 직전 턴은 보호한다. |
| `account_limits`, `context_state`, `model_catalog`, `availability` | 불필요 | 최신값 병합은 이력을 자르지 않는다. |
| `diagnostic` | 불필요 | |
| `control` | — | 정리 대상이 아니다. |

현재 모든 정리가 `history.go:115`의 단일 게이트 뒤에 있다. 범주별 정리는 그 범주의 조건만 본다. 진행 중 턴 동안의 일시적 초과는 허용하고, 허용된 초과분을 기록한다.

## 9. 기본값

현행 100MiB를 전체 기준으로 삼되, 범주 합이 그 값을 복제하지 않는다. 위 표의 기본 한도 합은 148MiB(+ latest 범주의 키 상한)이고 이 중 `conversation` 64MiB는 다른 범주의 초과로 소모되지 않는다. 총량 상한은 범주 합의 상한과 별개로 두지 않는다 — 각 범주가 자기 한도 안에 있으면 전체는 그 합 안에 있다.

측정된 부하에 대입하면: 본문 4~15KiB는 64MiB 안에서 사실상 무한하고, `work_detail` 20~27MiB는 32MiB 안에 들어가며, `account_limits`와 `model_catalog`는 수십 KiB로 떨어지고, `provider_reply`가 off이므로 raw는 대화 델타만 남아 16MiB 안에 들어간다. 즉 **같은 부하에서 trim이 대화 prefix에 닿지 않는다.**

미지정과 0을 구분한다: 미지정은 위 기본값, 명시적 0은 무제한, `disabled`는 모든 제거를 끈다(현행 `Policy.Disabled`와 같은 의미).

## 10. 전환

- 모드 변경은 **새 기록부터** 적용한다. 이미 디스크에 있는 기록을 자동으로 지우지 않고, RAM 이력을 자동으로 영속화하지 않는다.
- 적용 시점은 sequence와 generation으로 기록한다. 한 세션의 이력에서 어느 구간이 어떤 정책 아래 기록됐는지 답할 수 있어야 한다.
- 진행 중 턴과 미해결 제어 상태는 전환 중에도 보존한다.
- 예산 감소에 따른 기존 기록 정리는 명시적 요청으로만 수행한다.

### 기존 정책의 매핑

| 현행 | 새 모델 |
|---|---|
| `max_mib` (기본 100) | 범주 합의 기준값. 비례 배분하되 `conversation`은 줄이지 않는다. |
| `raw_mib` (기본 20) | `provider_stream` + `provider_reply`의 합. 기본 전환에서 `provider_reply`는 off이므로 전액 `provider_stream`으로 간다. |
| `disabled` | 모든 범주의 제거를 끈다. |
| `window_mib`, `window_turns` | 클라이언트 전용 선호로 그대로 유지한다. 서버 보존 정책과 다른 축이다. |

정책 파일이 없는 설치는 위 기본값으로 동작한다. 오래된 설정 파일은 계속 읽는다.

## 11. 이 문서가 고정하지 않는 것

- Recorder/Reader 경계의 구현(cxz#337)과 저장 구조(cxz#338, cxz#339).
- 정책 API의 wire 형태와 전파(cxz#340).
- 파생 저장소·검색·MCP의 적용(cxz#341)과 UI(cxz#342).
- 활성화 전후의 재측정(cxz#343).

범주 이름과 가용성 상태 이름은 여기서 확정된 것이며, 후속 단계는 이 이름을 쓴다.
