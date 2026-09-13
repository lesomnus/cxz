# CLI 입력 계약

## 영구 삭제

`cxz purge`는 현재 `--state` 설치의 영구 삭제 체크리스트다. 컨테이너, 프로젝트
대화/로그인 볼륨, 서버 DB/계정 볼륨, 도구 볼륨, 네트워크, 로컬 상태가 기본 선택된다.
Space/Enter로 체크를 바꾸고 ↑/↓ 또는 Tab으로 이동한다. PgUp/PgDn으로 정확한
대상 목록을 확인한 뒤 마지막 Confirm irreversible deletion 버튼에서 Enter로 실행한다.
Esc/Ctrl-C는 취소한다. 최소 화면 크기는 60 × 20이다.

`cxz purge --dry-run`은 비대화형에서도 삭제 없이 목록만 출력한다. 실제 삭제에는
대화형 터미널이 필요하며 --yes 우회는 없다. host/client에서 실행하고 native 서버/
에이전트는 먼저 종료해야 한다. Docker owner label과 리소스 ID를 확인한 뒤 manager,
프로젝트 컨테이너, 네트워크/볼륨, 로컬 파일 순서로 정리한다.

데이터를 남기는 선택이면 로컬 설치 정보도 남겨야 하며, 오류 시 설치 정보는 보존한다.
파일·볼륨 삭제는 백업 없이는 복구할 수 없다. 소스 워크스페이스, 개인 Claude/Codex
로그인 폴더, CLI 실행 파일, 공유 이미지/빌드 캐시, cxz owner label 없는 Docker 리소스,
알 수 없는 로컬 파일, 다른 설치 인스턴스는 삭제하지 않는다. 상태 디렉터리 자체는
남기고 알려진 하위 파일/디렉터리만 지운다. 공급자 측 OAuth 토큰 revoke는 하지 않는다.

## 명령 규칙

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

프로젝트에서 a는 독립 계정 화면, n/Ctrl-N은 새 세션용 계정 선택 화면을 연다.
계정이 없으면 앱을 종료하지 않고 n으로 추가한다. provider(←/→), alias, 선택적
display name을 Tab으로 이동하고 마지막 Create account에서 Enter로 등록한다.
계정 목록에서는 /로 검색(alias/name/provider/번호), ↑/↓로 선택, l로 로그인한다.
Claude는 현재 프로젝트, Codex 기본 backend는 중앙 로그인이며 기존 OAuth 흐름을
재사용한다. 로그인 성공/실패 후 화면으로 복귀한다. 등록과 로그인은 별개다.
Esc는 폼/검색을 닫고, 계정 목록에서 다시 누르면 프로젝트로 돌아간다.
Provider 이름은 Claude/Codex 브랜드색을 쓰고 6셀로 고정해 선택 화살표가 움직이지 않는다.

최상위 up은 준비 중 서버 체크포인트와 경과 시간을 stderr로 출력한다. devcontainer
이미지 pull/build 및 hook, 에이전트 설치, runtime 준비를 구분한다. 빠른 단계는
폴링 사이에 지나갈 수 있고, 오래 걸리는 단계는 10초마다 다시 표시한다. 진행률이나
상세 빌드 로그를 추정하지 않으며 --no-attach의 JSON stdout에는 섞이지 않는다.

최상위 up의 프로젝트 화면은 프로젝트 정보와 해당 프로젝트 세션만 보여준다.
↑/↓·Enter로 세션을 열고, n/Ctrl-N은 계정 선택·첫 로그인·새 세션 생성으로 이어진다.
프로젝트당 실행 세션 하나 제약은 유지하므로 기존 세션을 s로 중지한 뒤 새로 만든다.
d/Delete는 확인(y, Esc/n 취소) 후 선택한 세션을 중지·삭제한다. 확인 중 목록이 바뀌어도
처음 지정한 ID만 삭제한다. Ctrl-Q는 세션 화면에서 프로젝트 화면으로 돌아가며 agent를
중지하거나 프롬프트를 전송하지 않는다. Ctrl-C는 TUI만 닫는다.
it은 세션 화면을 선택해 열 뿐 중지된 agent를 자동 재개하지 않는다(Ctrl-R로 재개).

프로젝트 화면은 워크스페이스 정보와 선택 강조된 세션 카드 목록으로 구성된다.
세션 화면은 대화 본문, 승인/스크롤 상태, 여러 줄 입력창, 단축키 안내를 분리한다.
Enter/Alt-Enter/Ctrl-J는 줄바꿈, Ctrl-Enter/Ctrl-S는 전송, Ctrl-X는 입력 지우기다.
Ctrl-Enter는 CSI-u 또는 xterm modified-Enter를 보내는 터미널에서 구분된다. 기존 CR만
보내는 터미널에서는 Enter와 구분할 수 없으므로 Ctrl-S를 사용한다. Kitty 전체 키보드
프로토콜을 강제로 켜지는 않는다. 여러 줄 붙여넣기는 전송하지 않고 편집기에 남는다.
Esc는 대화 초안을 지우지 않는다. 선택/생성/승인 화면의 Enter 동작은 그대로다.
프로젝트로 돌아가거나 다른 세션을 선택해도 각 세션 초안은 현재 TUI 프로세스 안에서
유지되며, 종료 후에는 보존하지 않는다. PgUp/PgDn은 입력 커서를 움직이지 않고 본문만
스크롤한다. 최소 터미널 크기는 40 × 14이며 입력창/테두리/현재 입력 줄도 터미널
배경색을 유지한다. 입력창은 첫 줄 >, 다음 줄부터 어두운 1…9, 0, 1… 한 자리 번호를 표시한다.

브랜드 팔레트는 #24d17c / #031e2c / #aeff98 / #07898f이며 도구·진단에는
파스텔 보조색을 쓴다. Claude 이름은 테라코타색, Codex 이름은 검정 배경/흰 글자로
표시하며 대답 본문은 모두 밝은 흰색이다. 입력창 테두리는 좌우 마진 없이 터미널 너비를 채운다. 사용자 메시지는
YOU 대신 로컬 시간대의 MM-DD HH:mm 시각(흐린 색)과 > 접두사로 표시한다.
`>` 앞에는 화면 여백을 두지 않으며 에이전트 이름/답변에는 두 칸 들여쓰기를 둔다.
완료 JSON은 숨기고 응답 뒤 한 줄을 비운 다음 왼쪽 정렬된
요약을 표시한다. 순서는 완료 시각(MM-DD HH:MM) / 소요 시간, 비용, 토큰이다.
완료 시각은 이벤트 수신 시각의 로컬 시간대 표시다. 소요 시간은 00:00:00 ◷이며 0인 단위는
더 어둡게 표시한다. 비용은 $0.1처럼 축약하고 $0.10 미만은 ¢1.2처럼 센트로 표시한다.
0보다 크고 0.05센트보다 작은 비용은 ¢<.1로 표시하며 /usage의 USD 소수점 4자리는 유지한다. 토큰은 값 뒤에
↑ 입력, ↓ 출력, ↺ 캐시 읽기, ⊕ 캐시 생성, ∑ 전체 심볼을 표시한다.
제공되지 않은 항목은 생략하고 Codex 사용량은 tokenUsage.last만 사용한다.
제공된 소요 시간이 없으면 요청~완료 이벤트 간 시간을 ≈◷로 구분한다.
실패/중단은 안내를 유지하며 원본 JSON은 session events에서 조회할 수 있다.
완료 시각은 indicator 두 칸 뒤 바로 시작한다. 나머지 메트릭은 어두운 색의 5셀 고정 폭
(숫자 3칸 + 단위 2칸), 메트릭 간 1칸으로 배치한다. 비용은 통화 심볼이 앞에 있고 토큰 숫자는 우측 정렬한다.
토큰 수는 1k/1.2k/1m처럼 축약한다. 입력창 외에는 첫 두 칸을 indicator 전용으로
두고 일반 본문/도구/상태 안내를 두 칸 들여쓴다.
상단 세션 정보는 맨 아래 한 줄로 옮겼으며 대화 화면 하단의 상시 단축키는 숨긴다.
상태바에는 idle/working 등 상태를 표시하지 않고 working일 때만 대화 하단에 점자
스피너·진행 시간·Esc 중단 안내를 그린다. Esc를 누른 뒤 3초 내 다시 누르면 중단한다.
3초가 지나거나 턴이 바뀌면 다시 확인해야 하며 세션/run이 바뀌어도 확인은 승계하지 않는다.
starting/idle/working/waiting_input/stopping/stopped/interrupted/failed가
현재 상태 목록이다. 승인/답변 대기는 별도 박스, 준비/중단/오류는 안내로 유지한다.
`/help`는 상자형 로컬 오버레이로 표시한다. 서버/에이전트에는 전송하지 않는다.
↑/↓·PgUp/PgDn·마우스 휠로 내용을 스크롤하고 Esc로 닫는다. 대화 스크롤과 draft는
변경하지 않는다. /usage·/context도 같은 조회 오버레이를 사용한다.
도움말은 카테고리별 명령어 요약과 단축키로 나뉜다. `/help answer`, `/help permission`
처럼 명령어 이름을 붙이면 동작·주의사항·예제를 따로 보여준다. 키마다 한 칸 패딩을
두고 검정/짙은 회색 키 배경을 행마다 번갈아 표시한다. Enter는 줄바꿈, Ctrl+S는 전송이다.
/ 입력 시 대화 영역 하단에 상자형 명령어 힌트가 겹쳐 나타난다. 테두리 안에 최대
7개 항목만 표시한다. ↑/↓ 이동 시 가능한 범위에서 위아래 2항목을 미리 보여준다.
부분 문자 순서 기반 fuzzy 검색과 정확/접두사 우선 정렬, Tab 완성, Ctrl-Enter/Ctrl-S 실행,
Esc 닫기를 지원한다. /answer, /stop, /details 등도 힌트에 포함한다.
/context는 Claude 내장 보고서를 요청하거나 Codex의 마지막 tokenUsage.last와
modelContextWindow를 표시한다. 후자는 스냅샷이지 실시간 카테고리별 토큰 계산이 아니다.
Claude 응답은 해당 요청 client ID의 입력 echo를 확인한 뒤 같은 run의 응답만 모아
오버레이에 표시한다. 다른 입력이 시작되면 수집을 멈춘다. 닫은 창을 늦은 결과가 다시
열지 않는다. 조회 턴은 대화 렌더링에서 제외하지만 원본 저널에는 그대로 남긴다.
/compact는 idle 세션에서 Claude 내장 명령 또는 Codex thread/compact/start를 호출한다.
공급자 컨텍스트만 압축하며 cxz 원본 저널은 지우지 않는다. 완료/실패는 공급자 이벤트로 확인한다.
/usage는 전체 세션 저널을 페이지별로 조회해 토큰/비용/시간과 항목별 제공 턴 수를
집계한다. Claude total_cost_usd는 실행별 누적 증가분만, Codex 토큰은 last만 사용한다.
계정 한도나 실제 청구액이 아니며 미제공 값은 추정하지 않는다.

최하단 우측 계정 한도는 별도 provider 표시 모델로 추상화했다. 백분율과 점자 막대는
**남은 비율**, 5h/wk 등은 공급자가 보고한 창, 뒤 시간은 reset까지 남은 시간이다.
Codex account/rateLimits/read·updated, Claude get_usage·rate_limit_event를 실행 중인
공급자 프로세스로 조회한다. 시작/턴 완료/1분 간격으로 갱신하며 TUI가 OAuth 토큰을
읽거나 갱신하지 않는다. 조회 기록이 없으면 quota waiting, 요청 후 응답 대기는 polling,
미응답이 1분 이상인 것을 다음 조회 시 확인하면 timeout으로 표시하고 재시도한다.
미응답 요청이 있는 동안에는 중복 조회를 억제한다. `/usage`에 마지막 quota 요청 시각을
표시하므로 매분 폴링 여부를 확인할 수 있다. 공급자 메서드 미지원은 unsupported,
계정에서 한도 미제공은 unavailable, 조회 실패는 error로 표시한다. /usage에 원인별
안내와 스냅샷 관측/reset 시각을 덧붙인다. 상태바 오른쪽은 한 칸 비운다. 2분 이상 오래된 값은
~로 표시하며 오류/타임아웃 때도 이전 값에 ~를 붙인다. 계정 이름 앞 장식 심볼은 생략한다.
reset 시각이 지나도 100%로 추정하지 않고 refresh를 표시한다.
좁은 화면에서는 제목을 먼저 줄이고 추가 한도는 +로 표시한다.

### 승인·스크롤·대화 렌더링

접속 시 최근 128개 이벤트를 한 번에 읽어 마지막 화면부터 표시한다. 위쪽으로 스크롤하면
이전 기록을 128개씩 추가로 읽으며, 추가 전 보고 있던 줄의 위치를 유지한다. 화면을 채울
내용이 부족하면 이전 페이지를 추가로 읽는다. PgUp/Ctrl+Home/마우스 휠을 지원한다.
아직 전체 기록을 읽지 않았다면 줄 번호는 `loaded L`로 표시하며 전체 저널의 절대 줄
번호가 아니다. 읽은 페이지와 렌더링 캐시는 현재 TUI에서 유지한다. 이는 TUI의 지연
로딩이며 서버의 원본 저널 재생/SQLite projection 자체를 바꾼 것은 아니다.

Markdown fenced/indented 코드 블록은 본문 폭 전체의 검정 배경과 내부 한 칸 여백으로
표시한다. 도움말은 감지된 색 프로필을 표시한다. True Color에서는 홀수/짝수 키캡을
#000000/#101010, ` + ` 연결부를 #080808/#181818로 표시한다. 256색/16색에서는
키캡과 연결부를 모두 검정으로 통일한다. 감지는 TERM/COLORTERM/TTY 등 환경정보를
사용하므로 tmux/SSH가 실제 지원보다 낮은 정보를 전달할 수 있다.

표시 기준은 대화와 함께 봐야 하는지 여부다. /help·/usage·/context와 /model·/effort
선택, / 자동완성은 공간을 추가하지 않는 테두리 상자형 오버레이다. 대화와 함께
참고하는 /approval·/details·권한 변경 안내는 구분선이 있는 로컬 출력 영역으로 남긴다.
Pending approvals도 고정 공간을 차지하므로 외곽 상자 대신 수평 구분선을 사용한다.
/compact는 별도 화면 없이 기존 대화의 실행 결과 표시를 유지한다.
구분선은 좌우 두 칸을 비운다. 조회/선택 모달이 열리면 입력창 테두리·프롬프트·커서를
비활성 표시하며 닫으면 기존 포커스/draft로 돌아온다. / 자동완성은 입력 보조이므로
입력창 포커스를 유지한다.
활성 조회/선택 모달은 brand green(#24d17c) 테두리로 포커스를 표시한다. 입력 보조
자동완성 상자는 비활성 teal 테두리를 유지해 입력창이 포커스 대상임을 구별한다.

대화를 위로 스크롤했을 때 위치/시간 안내 바로 위의 빈 줄을 스크롤 막대로 사용한다.
어두운 ─ 선이 좌우 여백 없이 화면 전체를 채우고 ◆︎가 현재 위치를 나타낸다.
막대를 위해 행을 추가하지 않으므로 입력창/상태바 높이는 움직이지 않는다.
위치는 현재 로드한 대화의 스크롤 가능 범위를 기준으로 계산한다. 이전 페이지를
불러오면 범위도 갱신되며, 아직 읽지 않은 전체 저널의 절대 위치를 추정하지 않는다.

### 모델·추론 강도

`/model` 또는 `/effort`는 로컬 선택 오버레이를 연다. ↑/↓ 선택, Enter 적용,
Esc 취소, 하단 fuzzy 검색을 지원하며 최대 7개 선택지를 표시한다. 여는 동작은
읽기 전용이며 일반 채팅으로 전송하지 않는다. tail 밖에 있는 현재 run의 capability도
History에서 조회한다. 현재 run의 모델 제어 지원 기록이 없으면 업데이트 안내만
표시하고, 인자를 붙인 설정 명령도 전송하지 않는다.
`/model <id>` → `/effort <level>`로 설정하며 `/effort default`로 기본 강도로 복귀한다.
강도를 지정한 상태에서 모델을 바꾸려면 먼저 `/effort default`를 실행한다.
명령별 예제와 제한은 `/help model`, `/help effort`에 있다.

- 공통 ModelOption으로 Claude의 supportedEffortLevels와 Codex의
  supportedReasoningEfforts/defaultReasoningEffort를 변환한다. 모델/강도 목록을
  하드코딩하지 않고 공급자가 보고하지 않은 선택은 거절한다.
- Claude는 initialize의 모델 목록을 사용하고 set_model/apply_flag_settings로
  변경한다. 응답 전에는 다음 대화를 차단하며 거절을 성공으로 표시하지 않는다.
  flag settings로 전달할 수 없는 session-only max effort는 명시적으로 거절한다.
  모델 목록의 실시간 재조회 API는 확인되지 않아 초기화 스냅샷임을 표시한다.
- Codex는 [공식 app-server model/list](https://learn.chatgpt.com/docs/app-server#list-models-modellist)를
  시작 시·idle 매분 호출하고 모든 페이지를 합친다. TUI 선택기는 기록된 목록을 읽는다.
  선택은 다음 turn/start의
  model/effort에 전달한다. 기본값 복귀도 보고된 기본값을 사용한다.
- 실행 중인 CLI 버전/계정/공급자 정책에 따른 지원 범위를 유지한다. 확인된 선택은
  런타임 저널에 남아 resume 시 복원되고 TUI 상태바에 반영된다. 리소스의 최초 생성
  model 값과 실행 중 선택은 구분한다. 이전 프로세스에는 새 코드가 자동 적용되지 않는다.
- 실제 설치 Claude의 빈 프로필로 모델 선택·effort 반영·get_settings를 검증했다.
  사용자 로그인 계정이나 모델 호출은 하지 않았다. Codex 변경은 프로토콜 fixture로 검증했다.

### quota 진단 로그 공유

클라이언트만 교체해도 기존 manager/프로젝트 서버/supervisor의 코드는 그대로다.
일반 up은 살아 있는 프로젝트 서버를 재사용한다. 최신 로컬 바이너리를 설치하는
`./cxz install --recreate`는 manager와 공유 도구 바이너리를 갱신하지만 기존 프로젝트
프로세스는 종료하지 않는다. 이후 `./cxz project recreate .`로 프로젝트 런타임까지
교체할 수 있다. **프로젝트 recreate는 writable layer 삭제와 편집기 연결 해제를
수반한다.** 워크스페이스/named volume은 유지되지만 컨테이너 안에만 있는 파일은
먼저 보관해야 한다. 기존 세션이 중단 상태이면 TUI의 Ctrl+R로 재개한다.
quota 이벤트가 전혀 없고 /model이 입력 대화로 기록되는 조합은 구형 supervisor가
실행 중인 상황과 일치한다. 단순히 polling 주기를 기다리는 것으로 해결되지 않는다.

먼저 대화창에서 `/usage`를 실행해 Account quota 상태와 마지막 조회 시각을 확인한다.
추가 진단에는 아래처럼 **payload를 제외한** 이벤트 메타데이터만 추출한다(jq 필요).
SESSION_ALIAS를 해당 세션 alias로 바꾸고 약 70초 관찰한 뒤 Ctrl+C로 종료한다.

```sh
./cxz session events SESSION_ALIAS --format json |
  jq --unbuffered 'select(.kind == "usage" or .kind == "usage_status") | {seq, run_id, time_ms, kind, text}'
```

이 명령은 과거 기록을 먼저 출력하고 새 이벤트를 계속 구독한다. usage가 없으면 아무것도
출력되지 않을 수 있으며 그것도 진단에 유용하다. `/usage` 출력과 함께 공유하면 된다.
원본 session events에는 대화/명령/도구 결과가 들어 있으므로 통째로 공유하지 않는다.
인증 파일·access/refresh token은 공유하지 않는다. 서버 자체 오류는
`./cxz manager logs --tail 100`, 준비 단계 오류는 `./cxz project logs PROJECT --tail 100`으로
확인할 수 있으나 이 로그들도 공유 전에 비밀과 개인 경로를 검토해야 한다.

### 승인 영역

대화와 안내 줄 사이에 빈 줄을 둔다. 승인 영역은 안내와 입력창 사이에 공간을 차지하며
상하 구분선으로 구별한다. 오버레이가 아니므로 대화를 가리지 않는다.
Tab은 위→아래의 승인(있을 때)→입력→최하단 세션 선택을 순환하며 Shift-Tab은 역순이다.
승인 포커스에서는 ↑/↓ 선택,
Enter 승낙, Backspace 거절이다. 요청이 사라지거나 결정을 보낸 뒤에는 입력으로 돌아가
키 반복이 다음 요청을 승인하지 않게 한다. /approval로 선택 요청의 전체 payload를
대화에서 조회한다. 질문형 요청은 /answer JSON 답변이 필요하며 Enter로 답을 만들지 않는다.
승인 제목/명령/사유는 공급자별 구조를 공통 표시 모델로 변환한다. 본문은 두 칸 들여쓰고
원본 payload도 유지한다. 승인 박스에 포커스한 상태에서 PgUp/PgDn, Ctrl-Up/Down,
Ctrl-Home/End, 박스 위 마우스 휠로 내용을 스크롤한다. 나머지 때는 대화창을 스크롤한다.
승인 결과는 새 행이 아니라 원래 요청의 ☐를 🗹/☒로 바꾸며 허용/거절/취소 색을 구분한다.
tool_result는 상태·줄 수·크기를 한 줄로 요약한다. /details는 최신 결과 전체를 표시하고,
이전 결과는 session events의 원본 payload로 조회할 수 있다.

Claude 실행 시 --tools를 넘기지 않아 기본 도구를 사용한다. 내장 /context·/compact를
위해 --disable-slash-commands도 제거하되 설정 소스/MCP/hook 격리와 수동 승인 정책은
유지한다. 공급자별 profile 설정은 TODO.md에 후속 과제로 기록했다.

/permission full은 현재 보고 있는 세션의 run에 대해 이 TUI가 연결된 동안 알려진
도구·명령·파일·권한 요청을 자동 승인한다. 이미 대기 중인 요청도 포함한다. 질문과
미지원 요청은 수동으로 남긴다. /permission ask로 해제한다. 연결 단절, run 교체,
승인 오류, 프로젝트 복귀, 앱 종료 시 해제되며 서버/공급자 설정으로 저장하지 않는다.
정상 Reply RPC를 사용해 run/request identity 검사를 유지하고 이미 전송한 결정은
되돌리지 않는다. 모호한 실패는 자동 재시도하지 않는다.

PgUp/PgDn·마우스 휠로 스크롤하고 Ctrl+Home/End로 처음/최신 위치로 이동한다.
스크롤 중 L 시작–끝/전체와 해당 이벤트의 로컬 시각을 표시한다. 줄 번호는 현재
창 너비로 렌더링한 처음부터 계산하므로 resize 시 달라진다. 같은 이벤트의 여러 줄은
같은 시각이다. 과거를 읽을 때 새 출력이 위치를 바꾸지 않는다. 마지막 사용자 입력이
위로 완전히 사라지면 대화 최상단을 최대 2줄 덮어 고정한다. 고정 행은 indicator부터
오른쪽 빈 칸까지 `#031e2c` 배경을 채운다. 화면은 실제 터미널 크기와 창 크기 변경을
따른다. 최근 2,000개 제한을 제거해
현재 TUI에서 수신한 전체 세션 이벤트를 보유하므로 큰 세션은 메모리를 더 사용한다.

에이전트 응답의 시작은 indicator 열의 `• CLAUDE` / `• CODEX`로 구분한다.
대답 본문의 두 칸 들여쓰기는 유지한다. 기본 전송 안내는 Ctrl-S다. Ctrl-Enter는
터미널이 구분된 시퀀스를 보낼 때만 동작하며 cxz가 확장 키보드 모드를 자동 활성화하지는 않는다.

응답 프로토콜의 text에는 확정적 Markdown 형식 표시가 없다. 문법이 발견되면
CommonMark/GFM으로 렌더링한다. 제목/목록/표/강조/코드를 지원하고 백틱 코드 구간은
검정 배경이다. HTML·외부 이미지 fetch·OSC 링크 실행은 하지 않으며 원본 journal은 유지한다.
물리 터미널 커서를 위젯의 실제 커서 칸으로 맞춰 IME 후보/조합 위치를 보정한다.
줄바꿈/한글 셀 너비/스크롤/NO_COLOR를 고려하고 OAuth 화면에는 보정을 적용하지 않는다.
사용자 OS IME의 미완성 글자 조합은 사용자 터미널에서 재확인이 필요하다.
상태바는 선택 표시 1칸 + alias 7칸으로 시작하며 account 라벨은 ◉로 대체했다.
Tab으로 세션 선택 모드를 켠 뒤 r로 alias 편집, Enter 저장, Esc 취소한다. 오류 시
편집 상태를 유지한다. alias는 payday Session 필드/SQLite unique index로 저장하며
새 세션과 기존 세션 모두 3–7자 영단어가 무작위 할당된다. 직접 변경은 소문자 a–z
3–7자를 허용한다. session 명령에도 alias를 사용할 수 있다. 삭제 시 alias는 해제하고
저널은 보존한다. 단어 풀이 모두 사용 중이면 숫자 접미사를 만들지 않고 오류를 반환한다.
working/idle 등 state 이벤트는 본문에 누적하지 않고 현재 세션 상태 영역을 갱신한다.
원본 이벤트 저장과 재접속 cursor는 바꾸지 않는다.

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
