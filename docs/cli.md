# CLI 입력 계약

2026-09-12 전체 명령 점검 기준. 대문자는 필수 positional 인자이고 `[인자]`는
생략 가능하다. xli 규칙에 따라 옵션은 해당 명령 뒤, positional 인자 앞에 쓴다.
루트 옵션 `--state`는 명령 앞에 둔다. 도움말과 completion은 설치 없이 사용할 수 있다.

```sh
cxz account add --name "Company" codex work
cxz account login work
cxz session new --account work .
cxz manager update ghcr.io/lesomnus/cxz:edge
```

## 필수값과 기본값 점검

데이터·관리 명령은 `<리소스> <동사>` 형식이다. 워크스페이스 진입 경험을 위한
install/uninstall/up/down/it은 최상위에 둔다. projects/ls/new의 최상위 별칭은 없다.
version/completion/tui는 공통 도구다.
리소스 이름만 입력하면 도움말을 표시한다.

데이터 출력의 기본값은 표이며 `--format json`으로 전체 JSON을 받을 수 있다.
목록은 주요 열, 단일 객체는 FIELD/VALUE 표로 표시한다. 긴 셀은 줄이고 한글의
표시 폭을 고려한다. JSON은 기존 필드/목록 구조를 유지하며 표의 열 제한·문자열 축약을 적용하지 않는다.
이벤트 스트림의 JSON은 한 줄당 한 객체(JSONL)다.

```sh
cxz project ls
cxz session ls --format json
cxz account get --format table work
cxz --format json project ls
```

명령의 `--format`이 루트 설정보다 우선한다. scripts는 기본값에 의존하지 말고 json을
명시한다. shell/exec/logs/login/TUI는 원문/대화형 출력을 유지하며 format으로 변환하지
않는다. account status는 인증 파일 존재와 vendor 검증 여부를 구분해 출력한다.

| 명령 | 입력 계약 / 생략 시 동작 |
| --- | --- |
| `install` | `--image` 생략 시 현재 바이너리로 빌드. `--workspace-root`는 기존 설치/실행 환경에서 결정하며 결정 불가 시 오류. `--recreate` 기본 false |
| `uninstall` | 인자 없음. manager만 제거하고 프로젝트와 볼륨 보존 |
| `up [WORKSPACE]` | 기본 `.`. 프로젝트를 준비하고 프로젝트 화면을 연다. 이미 실행 중이면 재준비 없이 연결. 세션 생성/Account/로그인 요구 없음. 비대화형 또는 `--no-attach`는 프로젝트 정보 출력 |
| `down [WORKSPACE]` | 기본 `.`. 소유 컨테이너 정리 후 프로젝트·세션을 사용 목록에서 삭제. 워크스페이스 소스/이름 있는 볼륨/저널은 보존 |
| `it [WORKSPACE]` | 기본 `.`. 해당 프로젝트의 가장 최근 생성 세션 화면. 다른 프로젝트 세션은 제외하고 생성 시각 동률은 ID로 고정 정렬. 프로젝트/세션이 없으면 오류이며 자동 생성하지 않음 |
| `manager serve` | foreground 서버. `--agent`는 Claude 실행 파일 경로이며 기본 `claude`; 계정의 agent 선택이 아님 |
| `manager update IMAGE` | 이미지 필수. 명시적 tag/digest 필요, latest 거부 |
| `manager rollback`, `version` | 인자 없음. 이전 이미지로 manager 교체 / 버전 출력 |
| `account add AGENT ACCOUNT` | agent 필수(`claude`/`codex`), 계정 alias 필수. `--name`은 alias 기본값. `--auth-backend`는 Claude `project-local-oauth`, Codex `brokered-access-token` |
| `account ls`, `backend ls` | 인자 없음 |
| `account get ACCOUNT`, `binding ls ACCOUNT` | 계정 필수 |
| `account login ACCOUNT`, `account status ACCOUNT` | 계정 필수. 프로젝트별 OAuth는 `--project` 기본 현재 디렉터리. 중앙 인증은 프로젝트 불필요하며 명시적 `--project` 거부 |
| `project add PROJECT` | 등록 대상 필수. `--name`, `--alias`는 자동 결정 가능 |
| `project set PROJECT` | 대상 필수이며 `--name`, `--alias` 중 하나 이상 필수 |
| `session new [WORKSPACE]` | 경로 기본 `.`. 새 세션의 `--account`는 터미널에서 선택, 비대화형에서는 필수 |
| `project up [WORKSPACE]`, `project recreate [WORKSPACE]` | 경로 기본 `.`. 기존 세션 계정 유지; 새 세션이면 계정 선택/명시 필요. recreate는 대화형 확인 또는 `--yes` 필요 |
| `project down [WORKSPACE]` | 경로 기본 `.`. 컨테이너만 내리고 등록·세션은 유지하는 저수준 동작. 최상위 down과 다름 |
| `session attach [TARGET]` , `tui` (`watch`) | 대상 생략 시 선택/TUI. tui는 인자 없음 |
| `project shell PROJECT [COMMAND...]` | 대상 필수, 명령 기본 sh |
| `project exec PROJECT COMMAND...` | 대상과 실행 명령 모두 필수. `project exec PROJECT -- COMMAND...`로 옵션 전달 가능 |
| `session ls`, `project ls` | 인자 없음 |
| `session get SESSION`, `session interrupt SESSION`, `session resume SESSION`, `session stop SESSION` | 세션 필수 |
| `session send SESSION TEXT` | 세션과 비어 있지 않은 메시지 필수 |
| `session reply SESSION REQUEST DECISION [ANSWERS_JSON]` | decision은 allow/deny. 답변 JSON은 문자열 값의 object; 실제 질문별 필수 답변은 서버에서 검사 |
| `session events SESSION [AFTER_SEQ]` | 세션 필수, sequence 기본 0 (uint64) |
| `config show` | 인자 없이 현재 비밀 아닌 설정 출력 |
| `config set KEY VALUE`, `config unset KEY` | claude-model/codex-model만 설정. unset은 이전 agent 설정 제거도 허용 |
| `manager doctor` | 인자 없이 읽기 전용 진단 |
| `manager logs`, `project logs PROJECT` | manager/프로젝트 로그를 구분한다. `--tail` 기본 100, 범위 1–10000 |
| `completion SHELL` | xli 제공 shell completion 생성 |

new/up/recreate의 `--agent`는 Account/기존 세션에서 결정하므로 선택 옵션이며,
명시하면 일치 여부를 검사한다. `--model`은 새 세션의 agent별 설정 또는 vendor 기본값,
`--config`는 자동 탐색, `--name`/`--alias`는 프로젝트 기본값을 사용한다.
`--no-attach`, `--trust-config`, `--yes`는 명시적인 동작/신뢰/확인 스위치다.
조건부 필수인 옵션은 대화형 선택이나 재접속을 위해 positional로 바꾸지 않았다.

### 계정 선택 화면

최상위 up의 프로젝트 화면은 프로젝트 정보와 해당 프로젝트 세션만 보여준다.
↑/↓·Enter로 세션을 열고, n/Ctrl-N은 계정 선택·첫 로그인·새 세션 생성으로 이어진다.
프로젝트당 실행 세션 하나 제약은 유지하므로 기존 세션을 s로 중지한 뒤 새로 만든다.
d/Delete는 확인(y, Esc/n 취소) 후 선택한 세션을 중지·삭제한다. 확인 중 목록이 바뀌어도
처음 지정한 ID만 삭제한다. Ctrl-Q는 세션 화면에서 프로젝트 화면으로 돌아가며 agent를
중지하거나 프롬프트를 전송하지 않는다. Ctrl-C는 TUI만 닫는다.
it은 세션 화면을 선택해 열 뿐 중지된 agent를 자동 재개하지 않는다(Ctrl-R로 재개).

삭제는 payday Listed=false를 영속 tombstone으로 보관하는 논리 삭제다. 일반 목록·Get·
세션 제어에서 제외하며 재시작/상태 동기화가 복원하지 않는다. up으로 같은 workspace를
명시적으로 다시 등록할 수 있지만 이전 세션은 복원하지 않는다. 볼륨과 보관된 저널은
삭제하지 않는다. 복구 UI는 없으며 resources.db를 수동 제거·재구축하면 tombstone도
잃을 수 있으므로 resource DB를 포함해 백업해야 한다.

`session new`/`project up`/`project recreate`에서 새 세션의 계정 선택이 필요하면 검색 가능한 TUI를 연다.
목록에서 ↑/↓(Ctrl-P/Ctrl-N)로 이동하고 Enter로 확정한다. 하단 Search 입력창은
항상 입력 가능하며 번호·display name·alias의 정확한 일치를 우선하고, 없으면
이름·alias·agent의 부분 일치로 검색한다. 영문 대소문자는 구분하지 않는다.
번호는 검색 전 원래 목록 기준이며 중복 이름이나 번호/alias 충돌은 후보를 모두 표시한다.
검색 결과가 없으면 Enter로 진행하지 않는다. Esc/Ctrl-C는 선택을 취소한다.
비대화형 실행은 기존대로 `--account ALIAS`를 명시한다.

계정 선택 뒤 프로젝트별 OAuth의 첫 로그인이 필요하면 대화형 new/up/recreate는
준비된 컨테이너에서 공식 agent 로그인으로 이어지고, 성공 후 세션 연결을 계속한다.
로그인 실패/취소는 재시도하지 않고 프로젝트는 유지한다. 비대화형에서는 로그인 대기
대신 프로젝트를 명시한 로그인 명령을 안내한다. 기존 인증 손상과 중앙 인증 오류는
자동 로그인의 대상이 아니다. CLI뿐 아니라 프로젝트 runtime도 업데이트해야 한다.

## 내부 명령

- `_new-local ACCOUNT WORKSPACE [TITLE]`: 테스트용 로컬 세션 생성. 계정과 경로 필수.
- `_account-login`, `_account-import`, `_account-status`: `ACCOUNT AGENT [BACKEND]`.
  backend 생략 시 프로젝트별 OAuth. 계정·agent·backend는 실행 전에 검사한다.
- `_central-account-login ACCOUNT`, `_central-account-status ACCOUNT`: 중앙 인증 내부 진입점.
- `_supervise SESSION`, `_guard PID`: 각각 세션과 PID 필수.
- `_ready`, `_boot`, `_bridge`, `_project`: positional 인자 없음; runtime 환경에서 대상 결정.
- `_account-bind`: JSON stdin과 runtime 프로젝트 identity/capability를 검사한다.

## 변경 사항 및 검증 범위

`account add --agent ... ACCOUNT`, `manager update --image ...`, `_new-local --account ...`를
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
