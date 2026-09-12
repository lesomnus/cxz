# CLI 입력 계약

2026-09-12 전체 명령 점검 기준. 대문자는 필수 positional 인자이고 `[인자]`는
생략 가능하다. xli 규칙에 따라 옵션은 해당 명령 뒤, positional 인자 앞에 쓴다.
루트 옵션 `--state`는 명령 앞에 둔다. 도움말과 completion은 설치 없이 사용할 수 있다.

```sh
cxz account add --name "Company" codex work
cxz account login work
cxz new --account work .
cxz update ghcr.io/lesomnus/cxz:edge
```

## 필수값과 기본값 점검

| 명령 | 입력 계약 / 생략 시 동작 |
| --- | --- |
| `install` | `--image` 생략 시 현재 바이너리로 빌드. `--workspace-root`는 기존 설치/실행 환경에서 결정하며 결정 불가 시 오류. `--recreate` 기본 false |
| `uninstall` | 인자 없음. manager만 제거하고 프로젝트와 볼륨 보존 |
| `serve` | foreground 서버. `--agent`는 Claude 실행 파일 경로이며 기본 `claude`; 계정의 agent 선택이 아님 |
| `update IMAGE` | 이미지 필수. 명시적 tag/digest 필요, latest 거부 |
| `rollback`, `version` | 인자 없음. 이전 이미지로 manager 교체 / 버전 출력 |
| `account add AGENT ACCOUNT` | agent 필수(`claude`/`codex`), 계정 alias 필수. `--name`은 alias 기본값. `--auth-backend`는 Claude `project-local-oauth`, Codex `brokered-access-token` |
| `account list`, `account backends` | 인자 없음 |
| `account get ACCOUNT`, `account bindings ACCOUNT` | 계정 필수 |
| `account login ACCOUNT`, `account status ACCOUNT` | 계정 필수. 프로젝트별 OAuth는 `--project` 기본 현재 디렉터리. 중앙 인증은 프로젝트 불필요하며 명시적 `--project` 거부 |
| `project add PROJECT` | 등록 대상 필수. `--name`, `--alias`는 자동 결정 가능 |
| `project set PROJECT` | 대상 필수이며 `--name`, `--alias` 중 하나 이상 필수 |
| `new [WORKSPACE]` | 경로 기본 `.`. 새 세션의 `--account`는 터미널에서 선택, 비대화형에서는 필수 |
| `up [WORKSPACE]`, `recreate [WORKSPACE]` | 경로 기본 `.`. 기존 세션 계정 유지; 새 세션이면 계정 선택/명시 필요. recreate는 대화형 확인 또는 `--yes` 필요 |
| `down [WORKSPACE]` | 경로 기본 `.` |
| `attach [TARGET]` (`it`), `tui` (`watch`) | 대상 생략 시 선택/TUI. tui는 인자 없음 |
| `shell PROJECT [COMMAND...]` | 대상 필수, 명령 기본 sh |
| `exec PROJECT COMMAND...` | 대상과 실행 명령 모두 필수. `exec PROJECT -- COMMAND...`로 옵션 전달 가능 |
| `ls`, `projects` | 인자 없음 |
| `get SESSION`, `interrupt SESSION`, `resume SESSION`, `stop SESSION` | 세션 필수 |
| `send SESSION TEXT` | 세션과 비어 있지 않은 메시지 필수 |
| `reply SESSION REQUEST DECISION [ANSWERS_JSON]` | decision은 allow/deny. 답변 JSON은 문자열 값의 object; 실제 질문별 필수 답변은 서버에서 검사 |
| `events SESSION [AFTER_SEQ]` | 세션 필수, sequence 기본 0 (uint64) |
| `config` | 인자 없이 현재 비밀 아닌 설정 출력 |
| `config set KEY VALUE`, `config unset KEY` | claude-model/codex-model만 설정. unset은 이전 agent 설정 제거도 허용 |
| `doctor` | 인자 없이 읽기 전용 진단 |
| `logs [PROJECT]` | 생략 시 manager 로그. `--tail` 기본 100, 범위 1–10000 |
| `completion SHELL` | xli 제공 shell completion 생성 |

new/up/recreate의 `--agent`는 Account/기존 세션에서 결정하므로 선택 옵션이며,
명시하면 일치 여부를 검사한다. `--model`은 새 세션의 agent별 설정 또는 vendor 기본값,
`--config`는 자동 탐색, `--name`/`--alias`는 프로젝트 기본값을 사용한다.
`--no-attach`, `--trust-config`, `--yes`는 명시적인 동작/신뢰/확인 스위치다.
조건부 필수인 옵션은 대화형 선택이나 재접속을 위해 positional로 바꾸지 않았다.

## 내부 명령

- `_new-local ACCOUNT WORKSPACE [TITLE]`: 테스트용 로컬 세션 생성. 계정과 경로 필수.
- `_account-login`, `_account-import`, `_account-status`: `ACCOUNT AGENT [BACKEND]`.
  backend 생략 시 프로젝트별 OAuth. 계정·agent·backend는 실행 전에 검사한다.
- `_central-account-login ACCOUNT`, `_central-account-status ACCOUNT`: 중앙 인증 내부 진입점.
- `_supervise SESSION`, `_guard PID`: 각각 세션과 PID 필수.
- `_ready`, `_boot`, `_bridge`, `_project`: positional 인자 없음; runtime 환경에서 대상 결정.
- `_account-bind`: JSON stdin과 runtime 프로젝트 identity/capability를 검사한다.

## 변경 사항 및 검증 범위

`account add --agent ... ACCOUNT`, `update --image ...`, `_new-local --account ...`를
필수 positional로 교체했다. 빈 문자열은 누락된 값의 대용으로 받지 않는다.
agent enum, alias, 모델, 이름, 수정값 누락, config 키, 답변 JSON은 설정 파일 접근이나
연결 전에 검사한다. 리소스 존재/소유권/기존 세션과의 일치 등은 읽기 전용 조회 후 검사한다.

효과가 없던 `config set agent`는 거부한다. agent는 Account에 고정되므로 계정을 선택한다.
기존 설정은 `config unset agent`로 제거할 수 있다. 사용되지 않던 `--claude-config`,
루트 `--agent` 호환 옵션 및 항상 실패하던 `login PROJECT`/`_login`은 제거했다.
인증은 `account login ACCOUNT`를 사용한다. 이전 CLI 형식의 호환 별칭은 제공하지 않는다.

전체 하위 명령 help, agent completion, API 전달, 잘못된 입력의 무연결·state 미생성,
세션 lifecycle 통합 테스트를 검증한다. 실제 OAuth/유료 모델 호출은 CLI 입력 계약
변경의 검증 범위에 포함하지 않는다.
