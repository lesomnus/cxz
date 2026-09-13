# 구현 진행 상황

## 2026-09-13 — provider 공통 질문 dialog

- Claude AskUserQuestion의 input.questions와 Codex requestUserInput의 params.questions를
  agentview.Question으로 정규화했다. Claude 질문 문구/Codex ID를 reply key로 유지하며,
  단일·다중 선택, description, preview, Other 및 Codex secret 입력 정책을 표현한다.
- 신규 pending 질문은 focus 모달을 자동으로 연다. 방향키/Tab 이동, Space/Enter 선택,
  Other 직접 입력, Next/Back, 최종 Submit(또는 Ctrl+S), Esc/Cancel은 거절 없이 닫기.
  /answer 및 pending 질문 Enter로 재열기. 긴 내용은 PgUp/PgDn/wheel로 스크롤한다.
- 질문 로그/요약은 approval 대신 question requested/answered로 표시하며 큰 tool JSON은
  접는다. 원본 journal 및 /approval raw 조회는 유지한다. 기존 JSON 답변도 계속 지원한다.
- 전송은 기존 Reply API를 사용하며 session/run/request에 결합한다. 빈 답/중복 제출을
  막고 실패 시 폼과 답을 보존한다. 자동 재전송하지 않으며 취소해도 chat draft는 유지한다.
- 사용자 계정으로 실제 Claude/Codex 질문을 생성하지 않는다. provider fixture와
  TUI → Reply 요청 테스트로 단일/다중/직접 입력, 모달, 취소/재열기, stale/실패를 검증한다.
- `go test ./...`, `go test -race ./internal/tui ./internal/agentview`, `go vet ./...`,
  `git diff --check` 통과. CLI만 교체·재접속하면 기존 pending에도 적용된다.


## 2026-09-13 — quota bar 구간 색상·Kitty Ctrl+Enter

- 남은 quota ≤50% peach `#F5CA9A`, ≤30% coral `#F5AF98`, ≤15% pink-red
  `#F49BAA`. bar 8칸만 변경하며 수치/라벨/시간/폭은 유지한다.
- 기존 modified Enter 디코더에 Unix TUI의 Kitty disambiguation push/query/pop을
  연결했다. 응답의 flags를 확인하며 미응답은 지원 확인 불가로 남긴다.
  Ctrl+Enter/Ctrl+S는 전송, Enter는 줄바꿈. 모드에서 함께 바뀌는 Esc/Ctrl/Alt 및
  Shift+Tab도 기존 키 이벤트로 변환한다. 붙여넣기는 변환/전송하지 않는다.
- alternate screen 진입 뒤 push, 이탈 전 pop으로 tea.Exec 로그인/종료 시 모드를
  복원한다. 일반 터미널은 Ctrl+S를 유지하며 non-Unix에서는 모드를 요청하지 않는다.
- 검증: quota 경계/ANSI/레이아웃, 모드 진입·이탈 순서, PTY의 Kitty 키/조회 응답,
  분할 CSI·UTF-8·붙여넣기 회귀. 실제 사용자 터미널/tmux/IME 조합은 미검증.
- `go test ./...`, `go test -race ./internal/tui`, `go vet ./...`, `git diff --check` 통과.


## 2026-09-13 — 모델 목록 갱신·로그인 진행 표시·자동 승인 표시

- 재시작과 purge 차이: restart는 에이전트 Stop/Resume이며 살아 있는 project runtime을
  교체하지 않는다. Boot도 기존 소켓에 연결되면 종료한다. purge 후 신규 설치는
  프로세스/설정/로그인까지 초기화한다. purge 전 로그가 없어 이번 quota/effort 실패의
  정확한 원인은 확정하지 않았다. 데이터 삭제를 업데이트 방법으로 권장하지 않는다.
- 설치된 Claude 2.1.267의 `list_models` 제어를 확인하고 실제 빈 프로필 probe에서
  initialize(4개), list_models(4개), set_model, effort flag, get_usage 응답을 검증했다.
  사용자 계정 quota/모델 entitlement는 이 probe로 검증하지 않는다. 사용자 보고처럼
  restart 후 목록이 확장되면 initial catalog 시점 차이가 설명되지만 정확한 캐시/정책
  원인은 추정이다. 모델 목록은 계정/정책 영향을 받을 수 있다:
  https://code.claude.com/docs/en/model-config
- Claude 모델 목록을 시작 직후와 idle 1분 주기로 재조회하고 journal에 게시한다.
  열려 있는 선택창도 갱신하며 선택한 항목을 가능한 한 유지한다. 미지원 제어는 초기
  목록을 보존하고 재요청을 중단한다. 모델을 하드코딩하거나 검증 없이 허용하지 않는다.
- account 생성 폼의 ↑/↓를 Tab/Shift+Tab과 같은 필드 이동에 연결했다.
- Claude 코드 제출 후 로그인 대기 spinner/경과 시간을 표시한다. 로그인 완료 이후
  세션 준비/연결 RPC 대기도 stderr spinner로 표시한다. 구조화 stdout은 유지한다.
- full permission에서 자동 승인 가능한 요청은 pending 박스/요청 줄에 순간 노출하지
  않고 실제 resolution 이후 allowed만 표시한다. 질문/알 수 없는 도구는 수동으로 남으며
  승인 실패 시 full 해제 후 요청을 다시 표시한다. 미확인 결과를 allowed로 꾸미지 않는다.
- 오래된 polling 기록만 남았을 때 요청 후 1분이 지나면 TUI도 timeout으로 표시한다.
  서버 무응답 자체를 해결했다고 간주하지 않으며, /usage의 마지막 요청 시각을 유지한다.
- 폼 키 이동, spinner, 연결 진행 출력, 자동 승인 숨김/수동 fallback, quota 경과 시간,
  모델 갱신/미지원 fallback과 열린 dialog 갱신 회귀 테스트를 추가했다. 전체 Go 테스트,
  TUI/accounts/supervisor/CLI race, go vet, git diff --check 및 실제 Claude 무인증
  control probe를 통과했다. 사용자 세션/컨테이너는 중단·재생성하지 않았다.

이번 모델 갱신에는 **CLI와 project runtime/supervisor 업데이트가 모두 필요**하다.
기존 프로세스에 클라이언트 코드만 교체해도 새 제어가 생기는 것은 아니다.

## 2026-09-13 — 재시작 확인 버튼 오버레이

- `/restart`의 추가 confirm/cancel 명령 입력을 없애고 Confirm/Cancel 버튼 모달로
  바꿨다. 기본 선택은 Cancel이며 Tab/Shift+Tab/방향키로 전환, Enter로 실행,
  Esc/Ctrl+Q로 취소한다. 두 버튼 모두 마우스 클릭을 지원한다.
- 모달이 열리면 입력창 포커스 스타일/커서 표시를 끄고 활성 brand 테두리와 버튼
  선택 스타일을 표시한다. 입력/마우스 이벤트를 가로채 대화 입력·승인·스크롤로
  전달하지 않는다. 좁은 창에서는 버튼을 세로로 배치하며 렌더링과 클릭 좌표를 공유한다.
- 30초 제한은 제거하고 확인 시 session/run 일치 검사는 유지했다. 실제 재시작
  처리와 중복 실행/실패 방지는 그대로다. `/help restart`와 모델 설정 안내도 갱신했다.
- 기본 취소, 입력/draft·스크롤 보존, 커서/포커스 복원, 좁은 화면 버튼 표시·클릭,
  버튼 외 영역/마우스 release 무시 회귀 테스트를 추가했다. 전체 Go 테스트,
  TUI race/go vet, git diff --check를 통과했다. 클라이언트만 업데이트하면 된다.

## 2026-09-13 — 대화창 /restart

- `/restart` 후 30초 안에 `/restart confirm`을 제출하면 현재 세션 에이전트를
  Stop→Resume한다. `/restart cancel`은 확인을 취소한다. 확인은 session/run에
  묶이며 만료/실행 변경 시 거부한다. 진행 중 중복 실행도 차단한다.
- 중단 실패 시 Resume하지 않으며, 재개 실패는 상태 확인 안내와 함께 출력한다.
  각각 별도 idempotency key를 사용한다. 자동 승인 권한을 초기화하며 세션/인증/기록은
  유지한다. 컨테이너 재생성/서버 업데이트는 하지 않는다. 사용자 세션은 테스트 중
  실제로 중단하지 않았다.
- 자동완성, `/help restart`, 모델 제어 정보 누락 안내에 추가했다. 잘못된 인자를
  포함한 `/restart`는 항상 로컬 처리하며 에이전트 대화로 전달하지 않는다.
- 명령 라우팅·확인/취소·만료·세션/run 변경·중복 방지·Stop/Resume 실패·이미 중단된
  세션의 재개를 fake RPC로 검증했다. 전체 Go 테스트, TUI race/go vet,
  git diff --check를 통과했다. 클라이언트 전용 기능 추가다.

## 2026-09-13 — 스크롤바 최신 끝점·에이전트 재개 안내

- journal 마지막 이벤트가 아니라 viewport가 도달할 수 있는 마지막 상단 행의
  journal 좌표를 스크롤바 끝점으로 사용한다. 화면 아래 이미 보이는 영역/숨겨진
  tail 이벤트를 이동 가능 거리로 포함해 작은 휠 이동에 핸들이 크게 뛰던 문제를 수정했다.
  과거 페이지 로딩 시 journal 좌표 고정 방식과 좌우 선 색상 구분은 유지한다.
- 574줄/높이 64에서 3줄 위로 이동, 숨겨진 tail 이벤트, 높이 변경 회귀 테스트를
  추가했다. 모델 제어 capability 누락 안내에 session stop/resume과 중단 후 Ctrl+R을
  명시했다. 실행 중인 사용자 세션은 중단/재시작하지 않았다.
  전체 Go 테스트, TUI race/go vet, git diff --check를 통과했다. 이번 수정은
  클라이언트 전용이며 capability 기록 누락의 실제 서버 원인을 확정한 것은 아니다.

## 2026-09-13 — 지연 로딩 스크롤바 위치 안정화

- 전체 화면 줄 수는 Markdown/숨김 이벤트/터미널 폭에 따라 달라 전체 렌더링 전에는
  알 수 없다. loaded line 비율 대신 이미 알려진 journal 전체 sequence 범위와
  각 표시 행의 event 좌표를 이용해 스크롤바 위치를 계산한다. 긴 응답 내부는 행
  비율로 보간한다. 실제 줄 길이 비율이 아닌 기록상 위치임을 status에 표시한다.
- 과거 페이지 prepend로 표시 중인 내용의 journal 좌표가 바뀌지 않으므로 로딩에
  따른 핸들 좌우 튐을 없앴다. 시작/최신 끝점과 전체 폭은 유지한다. 핸들 왼쪽은
  muted 밝기, 오른쪽은 기존 어두운 zeroStyle로 구분한다.
- 연속 두 페이지 로딩 중 위치 보존, 위 방향 이동의 단조성, 시작/끝점,
  좌우 색상 분리 테스트를 추가했다. 전체 Go 테스트, TUI race/go vet,
  git diff --check를 통과했다. 클라이언트 전용 변경으로 서버 재생성은 필요 없다.

## 2026-09-13 — 동시 세션·세션별 인증/실행 공간

- client 선택, manager Open/Resume, runtime Create의 workspace 단일 live 세션
  제한을 제거했다. 기존 세션은 provider/account가 일치할 때만 선택하며 explicit new는
  같은 생성 요청 재시도 외에는 새 세션을 만든다. 작업 트리/동시 편집 결정은 에이전트에 맡긴다.
- Claude와 명시적 로컬 OAuth Codex는 새 세션마다 공식 CLI에서 독립 로그인한다.
  project 공용 refresh token을 복제/공유하지 않는다. 중앙 Codex의 공급 권한과
  갱신 주체는 유지하되 각 app-server의 CODEX_HOME/기록은 별도 profile에 둔다.
- 생성 요청 키로 고정된 session-profiles 경로에 config/HOME/cache/tmp/runtime을
  분리했다. 재접속/재시작에도 경로는 유지된다. supervisor 중복 방지와 SIGKILL
  guard는 유지하며, workspace lease 대신 세션 profile/login lease를 가진다.
- 대화형 생성은 로그인 후 같은 생성 요청을 한 번 재시도한다. 재로그인은
  `cxz account login --session SESSION_ALIAS ACCOUNT`, 확인은 `account status`의
  같은 플래그다. 다른 세션은 중단할 필요가 없다. 재개 오류의 원래 생성 키도 전달한다.
- 이전 공용 provider profile의 기록/인증은 자동 이관하지 않는다. 기존 journal은
  보존되며 이전 버전 세션은 새 세션에서 별도 로그인해야 한다. CLI/manager/runtime
  모두 업데이트가 필요하다. 사용자 컨테이너는 이 작업에서 재생성하지 않았다.
- 검증: 전체 Go 테스트와 go vet 통과. fixture agent 프로세스 2개 동시 실행,
  독립 인증 요구, 한쪽 중단의 영향 없음, 다른 세션 실행 중 재개, 같은 세션의
  중복 프로세스 거부, 기존 recovery 테스트를 확인했다. CLI session-key 파싱과
  resume 로그인 대상 키 전달도 검증했다.
  실제 사용자 OAuth 로그인/토큰 만료 후 갱신은 사용자 인증이 필요해 미검증이다.
  종료 응답 직후 즉시 재개 시 guard/profile 잠금 해제를 기다리는 처리를 추가했다.
  주요 패키지 race 검사와 자식 프로세스까지 race로 빌드한 lifecycle 테스트
  2회 반복을 통과했다. 최종 전체 테스트, go vet, git diff --check도 통과했다.

## 2026-09-13 — 모달 활성 테두리·동시 세션 제한 확인

- 조회/모델 선택 모달에 활성 brand green 테두리를 적용했다. 자동완성은 입력창이
  포커스를 유지하므로 기존 teal 테두리를 사용한다. 테두리 색만 바꾸며 크기는 유지한다.
  전체 Go 테스트, TUI race, go vet, git diff --check와 모달 테두리 회귀 테스트를 통과했다.
- 세션 추가 거절은 provider 자체 오류가 아니라 cxz의 workspace 단일 live 세션 정책이다.
  client preflight, manager, runtime Create, supervisor의 workspace flock을 확인했다.
  Claude/Codex/혼합에 동일하게 적용되며 idle도 live다. 기존 계획의 동시 작업 트리 vs
  worktree 결정 유보에 해당한다. 이번 요청에서는 원인 확인만 하고 제한은 해제하지 않았다.

## 2026-09-13 — 모달 포커스 표시·대칭 구분선·스크롤 위치 막대

- 조회/모델 선택 모달에서는 composer 복사본을 blur해 표시한다. 원래 입력 포커스와
  draft는 유지하므로 닫을 때 바로 복원된다. 모달 중 composer IME 커서 anchor는 끈다.
  입력 보조 자동완성은 입력 포커스를 유지한다.
- 로컬 출력/Pending approvals 구분선의 좌우 여백을 두 칸으로 통일했다.
  좁은 화면의 Markdown 구분선도 오른쪽 여백을 침범하지 않게 했다.
- 스크롤 위치·시각 안내 바로 위의 기존 spacer 행에 full-width ─/◆︎ 막대를 표시한다.
  로드된 범위 기준이며 첫/끝 위치, 좁은 화면, 행 높이 유지 테스트를 추가했다.
- 모달 열기/닫기 시 테두리 및 커서 anchor, 포커스/draft 보존, 구분선 대칭 폭을
  회귀 테스트로 검증했다. 이번 변경은 클라이언트 전용이다.
  전체 Go 테스트, TUI race, go vet, git diff --check를 통과했다.

## 2026-09-13 — 상자형 오버레이와 구분선 영역 통일

- /help·/usage·/context 조회를 스크롤 가능한 공통 상자형 오버레이로 전환했다.
  /model·/effort 선택과 / 자동완성도 같은 테두리 합성 함수를 사용한다.
  Esc는 닫기, PgUp/PgDn/방향키/휠은 조회 영역 스크롤이며 대화 위치/draft는 유지한다.
- /approval·/details와 권한 변경 안내는 구분선이 있는 로컬 출력으로 유지했다.
  공간을 차지하는 Pending approvals는 외곽 상자 대신 수평 구분선을 사용한다.
- Claude /context는 요청 client ID echo 및 run을 확인해 해당 턴의 텍스트만 수집한다.
  다른 입력/종료/연결 해제를 처리하며, 늦은 결과는 닫힌 오버레이를 다시 열지 않는다.
  원본 journal은 유지하고 조회 턴은 대화 표시에서 제외한다. 이전 페이지에 입력 경계가
  아직 로드되지 않은 경우에는 해당 경계를 읽기 전까지 응답을 임의로 숨기지 않는다.
- 로컬 조회 경계는 TUI 변경이므로 이번 변경만을 위해 서버/컨테이너 recreate는 필요 없다.
  실제 Claude 로그인 실행은 이번에 하지 않았고 응답 상관관계는 프로토콜 fixture로 검증했다.
- 전체 Go 테스트, TUI race, go vet, git diff --check를 통과했다. 오버레이 테두리/폭,
  독립 스크롤과 draft 보존, 닫힌 usage 창, Claude 요청 echo 상관관계를 검증했다.

## 2026-09-13 — 구형 런타임 보호·로컬 모델 선택기·색 깊이별 키캡

- 이전 /model 구현은 인자 없는 조회도 Send로 전달했다. 구형 supervisor는 이것을
  일반 입력으로 처리할 수 있었다. 이제 /model·/effort는 읽기 전용 로컬 선택기를 열고
  현재 run의 capability 기록 확인 없이는 인자 있는 명령도 보내지 않는다.
  tail 밖 capability도 History로 읽는다. 검색/방향키/Enter/Esc, 세대·run 검증,
  닫기 시 조회 취소를 추가했다.
- True Color에서만 키캡 행과 + 연결부를 아주 어두운 서로 다른 색으로 표시한다.
  16/256색에서는 모두 검정이다. /help에 감지 프로필을 표시하고 모든 로컬 출력
  블록 위에 구분선을 추가했다.
- install의 공유 바이너리 atomic 교체와 Boot의 기존 소켓 재사용을 확인했다.
  클라이언트 교체만으로 서버가 갱신되지 않으며 install --recreate만으로도 기존
  프로젝트 프로세스는 남는다. 프로젝트 recreate의 데이터 보존 범위/손실 경고와
  전체 적용 순서를 docs/cli.md에 보강했다. 사용자 컨테이너는 변경하지 않았다.
- 구형 capability 부재/명령 미전송, 선택 시에만 전송, stale run 거절, 색 깊이별
  배경/연결부, 로컬 구분선 회귀 테스트를 추가했다.
  전체 Go 테스트, TUI race, go vet, git diff --check를 통과했다.

## 2026-09-13 — 최근 기록 우선 로딩·코드 블록·모델 제어

- 최초 접속은 최근 128개 이벤트를 일괄 표시하고, 위쪽 스크롤 시 128개씩 이전
  페이지를 불러온다. prepend 후 스크롤 앵커를 유지한다. 서버 재생/저널 전체 스캔과
  이미 읽은 페이지의 메모리 유지까지 제거한 완전한 가상화는 아니다.
- fenced/indented 코드 블록에 전체 폭 검정 배경과 내부 한 칸 여백을 적용했다.
  키캡 짝수 행 대비를 #303030으로 높이고 ANSI256/16색 폴백을 명시했다.
- /model, /effort 및 상세 도움말을 추가했다. 공통 모델 옵션으로 공급자별 모델/강도
  목록을 변환한다. Claude는 초기화 목록과 제어 응답 확인, Codex는 페이지가 있는
  model/list 주기 갱신과 다음 turn/start override를 사용한다. 선택은 저널로 복원한다.
  Claude 모델 목록 live refresh와 flag 설정으로 불가능한 max effort는 지원으로
  가장하지 않는다. 초기화 스냅샷/지원 불가를 표시한다.
- Codex 공식 app-server 문서와 실제 설치된 Claude CLI 프로토콜을 확인했다.
  격리된 빈 프로필 probe에서 4개 모델을 읽고 모델/effort 변경 승인 및 get_settings
  반영을 확인했다. 사용자 인증 계정/실제 모델 호출은 하지 않았다.
- quota waiting의 사용자 환경 원인은 미확정이다. /usage와 payload 제외 이벤트
  메타데이터를 안전하게 공유하는 명령, manager/project 로그 위치를 docs/cli.md에 적었다.
- 전체 Go 테스트(통합 포함), TUI/supervisor race, go vet, 초기 tail/prepend 앵커,
  코드 블록 폭/배경, 16색 키캡 구분, 공급자 설정 유효성/거절/중복/다음 턴 파라미터,
  모델 목록 pagination 검증을 수행했다.

## 2026-09-13 — 카테고리 도움말·키캡·quota 폴링 진단

- Ctrl+S 전송 / Enter 줄바꿈을 유지했다. /help는 카테고리별 요약과 단축키,
  /help answer 등은 명령별 동작·주의사항·예제로 분리했다. 모든 slash 명령을
  문서화하고 상세 도움말 인자가 유지되며 에이전트로 전송되지 않도록 검증했다.
- 키마다 한 칸 패딩과 밝은 글자, 홀수/짝수 단축키 행에 #000000/#151515
  키캡 배경을 적용했다. 상태 바 계정 이름 앞 심볼을 제거했다.
- 기존 quota 조회는 초기화 직후·턴 완료·활성 상태에서 매분 실행되는 것이 맞다.
  미응답/쓰기 오류가 관측되지 않는 경로를 보강했다. 조회 요청을 저널에 기록하고
  미응답 동안 중복 조회를 억제하며, 1분 이상 미응답을 다음 조회 시 timeout으로
  기록하고 재시도한다. 대화 상태나 OAuth 토큰을 변경하지 않는다.
- waiting(조회 기록 없음), polling(응답 대기), timeout을 구분하고 /usage에
  마지막 조회 시각을 추가했다. 응답 수신 시 정상 상태로 복구한다.
- 전체 Go 테스트, TUI/supervisor race, go vet, PTY 화면 이동, 키캡 색/패딩,
  도움말 폭/예제, 공급자별 미응답·재시도·복구 검증을 통과했다.
  실제 설치 Claude의 격리된 빈 프로필 get_usage probe도 통과했으며
  rate_limits_available=false였다. 사용자 로그인 계정 한도는 검증하지 못했다.
  현재 Docker 엔진에 실행 중인 컨테이너가 없어 사용자가 본 waiting 원인을 확정하지
  않았다. 기존 supervisor는 실행 코드를 유지하므로 업데이트 후 agent 재시작이 필요하다.

## 2026-09-12 — 응답 시작 표시·사용량 부재 진단·전송 키 안내

- 에이전트 헤더에 indicator 열의 •를 추가하고 답변 본문의 두 칸 들여쓰기는 유지했다.
  최하단 우측 사용량에는 한 칸 여백을 추가했다.
- quota waiting/unsupported/unavailable/error를 구분하고 /usage에 원인별 안내를
  추가했다. 기존 실행 프로세스는 새 cxz 바이너리로 자동 교체되지 않으므로, telemetry가
  계속 없으면 서버/런타임을 업데이트하고 agent를 재시작해야 한다.
- 이전 검증은 fixture였다. 이번에는 실제 설치된 Claude Code 2.1.267을 빈 프로필로
  실행해 get_usage 지원과 응답 필드를 확인했다. 인증·프롬프트·모델 호출은 하지 않았으며
  사용자 로그인 계정의 실제 quota는 확인하지 못했다. opt-in 재현 명령:
  `CXZ_PROBE_CLAUDE=1 go test ./internal/supervisor -run TestInstalledClaudeQuotaControl -v`.
- 실제 빈 프로필 응답은 rate_limits_available=false였다. 전체 Go 테스트(통합 포함),
  TUI/supervisor race, go vet, 표시/한도 상태 전이 회귀 검증을 통과했다.
- Ctrl-Enter의 legacy CR 중복과 확장 모드 opt-in 요구를 확인했다. 현재 cxz는 확장 모드를
  활성화하지 않으며, 기존에 구현/PTY 검증한 Ctrl-S를 기본 전송 안내로 강조한다.
  [키보드 프로토콜](https://sw.kovidgoyal.net/kitty/keyboard-protocol/) 참고.

## 2026-09-12 — 입력·명령 팔레트·승인·공급자 사용량/컨텍스트

- Enter 줄바꿈, Ctrl-Enter(CSI-u/xterm)/Ctrl-S 전송. 실제 PTY 키 디코딩과 붙여넣기
  비전송을 검증했다. Tab/Shift-Tab은 승인→입력→하단 세션의 정방향/역방향이다.
- 명령 팔레트는 위 빈 줄, 최대 7항목, 양쪽 2항목 scroll margin, fuzzy 검색을 적용했다.
- 응답 메트릭 앞 완료 시각을 추가하고 working에 경과 시간/Esc 안내, 3초 내 두 번째
  Esc 중단 확인을 구현했다. 확인은 새 턴으로 승계하지 않는다.
- internal/agentview에 공급자 승인/한도 표시 모델을 분리했다. 승인 내용 전체 스크롤,
  원래 요청 행의 결정 색/체크박스 갱신, 결과 한 줄 요약과 /details를 구현했다.
- 하단 우측은 남은 quota/막대/window/reset을 표시한다. Codex RPC 및 Claude 실험적
  get_usage와 rate_limit_event를 같은 agent 프로세스로 조회한다. 1분 갱신, 미지원/오래된
  스냅샷/기한 경과를 구분하며 사용량을 추정하거나 TUI에서 OAuth를 갱신하지 않는다.
- /context: Claude 내장 보고서, Codex 마지막 토큰 footprint/window. /compact: Claude
  내장 명령 및 Codex thread/compact/start. 기존 Send의 run/client ID·저널·재전송 방지를
  유지하고 실제 압축 완료 이벤트를 보관한다. 저널 자체는 압축/삭제하지 않는다.
- Claude --tools/--disable-slash-commands를 제거했다. 기존 수동 승인·설정/MCP/hook
  격리는 유지하고 도구 profile 설정은 TODO.md에 남겼다.
- 전체 Go 테스트(통합 포함), 공급자 정규화/프로토콜 단위 테스트 통과. 실제 유료 모델
  compaction/OAuth는 실행하지 않았으며 공급자 버전에 따른 quota 미지원은 안전하게 처리한다.
- PTY에서 Enter/Ctrl-Enter/Shift-Tab/Esc, 긴 한글·emoji paste와 모든 키 시퀀스 분할
  경계를 검사했다. Bubble Tea의 borrowed unknown-CSI 버퍼를 읽지 않도록 Unix 입력
  단계에서 키를 변환하며, 파일 descriptor·raw mode·취소 처리를 유지한다.
- TUI/supervisor/agentview race 및 go vet 통과. native command 통합 fixture에서
  payday Send→supervisor→대화/압축/사용량 이벤트와 압축 후 기존 저널 보존을 검증했다.
- 구현 커밋 `0e13367`. 별도 검증 커밋에 Claude/Codex 프로세스 fixture의 quota/compact
  응답과 lifecycle의 /context·/compact·저널 보존 검사를 포함했다.
- 참고: [Codex App Server](https://learn.chatgpt.com/docs/app-server),
  [Claude SDK commands](https://code.claude.com/docs/en/agent-sdk/slash-commands),
  @anthropic-ai/claude-agent-sdk 0.3.268의 SDKControlGetUsageResponse/SDKRateLimitInfo 타입.

## 2026-09-12 — 전체 화면 크기 감지 복구·고정 입력 배경

- IME 커서 보정용 출력 래퍼가 `term.File` 인터페이스를 숨겨 초기 크기 조회와
  SIGWINCH 구독이 생략되던 회귀를 수정했다. 파일 인터페이스를 전달하고 파일이
  아닌 출력에는 유효하지 않은 descriptor를 반환한다.
- 화면 위에 고정된 마지막 입력 최대 2줄은 indicator와 오른쪽 빈 칸까지
  `#031e2c` 배경으로 채운다. 일반 대화와 입력창 배경은 바꾸지 않는다.
- 실제 PTY 초기 117×37, SIGWINCH 확대 143×45·축소 63×19에서 모델과 렌더링
  크기가 일치하는 회귀 테스트를 추가했다. 한글 포함 고정 행의 전체 너비 배경과
  고정 해제 시 배경 제거도 검증한다.
- `go test ./...`, `go vet ./...`, TUI race 테스트 및 새 회귀 테스트 20회 반복 통과.

## 2026-09-12 — 승인 포커스·자동 승인·스크롤·Markdown·IME 입력

- 계정 Provider는 브랜드색+6셀 슬롯을 사용한다. Claude 코드 입력은 최대 3개 별,
  최소 3자리 Unicode 글자 수, 어두운 padding 0, 깜빡이는 커서를 표시한다.
- 상태바의 상태 배지를 제거하고 working에만 대화 점자 스피너를 표시한다.
  대화/안내 사이 빈 줄과 별도 승인 박스를 추가했다. Tab 포커스, ↑/↓ 선택,
  Enter/Backspace 결정, /approval 상세 조회와 질문의 /answer 요구를 구현했다.
- /permission full|ask는 현재 TUI·세션 run 범위로 정상 승인 RPC를 자동 전송한다.
  알려진 요청만 허용하고 질문/미지원 요청, stale run과 중복 전송을 제외한다.
  연결/승인 실패·프로젝트 복귀·run 교체에 수동 모드로 복귀하며 공급자 설정은 바꾸지 않는다.
- 마우스/PgUp/PgDn, Ctrl+Home/End, 렌더링 줄 번호·이벤트 시각, 스크롤 위치 유지,
  마지막 입력 최대 2줄 고정을 추가했다. 전체 수신 이벤트를 보유하며 응답 렌더링을 캐시한다.
- 공식 Claude TextBlock/Codex agentMessage 타입과 어댑터를 확인했다. Markdown MIME
  구분 필드가 없어 Goldmark CommonMark/GFM 구문 감지 후 로컬 렌더링한다.
  코드만 검정 배경이며 escape/HTML/외부 이미지 fetch는 허용하지 않는다.
- Bubble Tea v1의 마지막 줄 물리 커서 위치를 위젯 커서 칸으로 보정했다. Unicode,
  줄바꿈/스크롤, NO_COLOR, alt-screen 진입/복귀를 다루고 로그인 터미널은 간섭하지 않는다.
  사용자 OS IME 조합 자체는 여기서 재현하지 못해 실제 사용 터미널 확인이 필요하다.
- 전체 Go 테스트·vet, TUI/accounts/CLI race, 통합 테스트 및 PTY 화면 이동/로그인
  비밀 비노출 검증을 통과했다. 실제 OAuth·유료 모델 호출은 실행하지 않았다.

## 2026-09-12 — 입력창·메트릭 밀도, up 진행 상황, 계정 화면

- 입력창/테두리와 위젯 기본 현재 줄 배경을 제거했다. prompt gutter는 두 칸이며
  첫 줄 > 이후 1…9, 0, 1…로 순환해 긴 입력에서도 늘어나지 않는다.
- duration은 두 칸 들여쓰기 직후 시작한다. 나머지는 숫자 3칸+단위 2칸,
  메트릭 간 1칸으로 압축했다. 작은 비용은 센트(¢), 상세 USD 합계는 /usage로 보존한다.
- 최상위 up은 서버에 저장된 준비 체크포인트와 경과 시간을 stderr에 표시한다.
  devcontainer pull/build/hooks, 선택된 agent 설치, runtime boot/readiness 등을 구분한다.
  폴링 방식으로 빠른 단계는 건너뛸 수 있으며 진행률이나 상세 빌드 로그를 추정하지 않는다.
- 프로젝트 a는 계정 화면, n은 같은 화면의 새 세션 모드다. 빈 목록에서도 provider/
  alias/display name을 추가하고 검색·선택·로그인할 수 있다. Add는 payday API,
  로그인은 기존 중앙/프로젝트별 OAuth 흐름을 재사용하고 앱으로 복귀한다.
- 전체 Go 테스트·vet, CLI/TUI race, PTY 프로젝트→계정→추가→복귀,
  투명 배경 ANSI/숫자 순환/메트릭 위치/계정 검증·오류·검색/준비 단계 표시를 검증했다.
  실제 사용자 계정 OAuth와 유료 에이전트 호출, 실제 devcontainer 재생성은 실행하지 않았다.

## 2026-09-12 — 선택형 영구 데이터 삭제

- 최상위 cxz purge와 읽기 전용 --dry-run을 추가했다. 체크박스는 모두 선택된 상태로
  시작하고 마지막 Confirm 버튼에서만 삭제한다. 작은 터미널에서 확인 버튼이 숨겨지면
  삭제를 허용하지 않는다.
- 현재 --state의 owner로 컨테이너/프로젝트 볼륨/서버 볼륨/도구 볼륨/네트워크를
  수집하고 알려진 로컬 상태 파일을 개별 삭제한다. 소스·개인 에이전트 로그인·이미지/
  빌드 캐시·unlabeled Docker 리소스·알 수 없는 파일과 다른 설치는 건드리지 않는다.
- owner/리소스 식별자/생성 시각/로컬 inode 재검증, 광범위 경로·symlink 거부,
  실행 중인 native 프로세스 lock 검사, manager 우선 제거, 잔존 리소스 발견 시
  locator 보존을 적용했다. 볼륨 삭제는 강제하지 않으며 global prune은 사용하지 않는다.
- 가짜 Docker와 임시 상태로 전체/부분 선택, 소유권 변경, 중간 실패, 활성 daemon/
  supervisor, dry-run/TTY 제한, 체크박스/확인/취소를 검증했다.
  전체 Go 테스트·vet와 purge/TUI/CLI race 검사를 통과했다. 실제 설치 데이터는 삭제하지 않았다.

## 2026-09-12 — 영단어 세션 alias와 /usage

- 검정 배경을 입력창/테두리로 한정하고 입력창 줄번호를 첫 줄 >, 이후 1부터 표시한다.
  상태바는 선택 indicator 1칸과 alias 7칸을 먼저 표시하며 계정은 ◉ 심볼로 구분한다.
- payday Session.alias와 unique index, 생성기/기존 세션 backfill, alias 전용 Patch를
  추가하고 proto→payday→ent 및 내부 뷰 모델을 재생성했다. 3–7자 영단어를 무작위로
  할당하며 직접 편집은 소문자 3–7자를 검증한다. 삭제 시 alias만 해제한다.
- Tab 선택 모드의 r은 하단 alias 커서를 활성화한다. Enter는 저장 후 선택 모드로,
  Esc는 취소하며 중복/길이 오류는 편집기를 유지한다. CLI에도 alias 조회/제어를 연결했다.
- /usage는 전체 저널을 페이지별로 읽어 현재 세션 사용량을 집계한다. 제공 항목 수를
  표시하고 Claude 실행별 누적 비용 중복 합산과 Codex 누적 토큰 오표시를 막는다.
- 기존 SQLite 행의 alias 컬럼 없는 스키마에서 실제 업그레이드/backfill, 재시작 후
  rename 유지, 중복/보호 필드 검사, 고정 폭 편집, 줄번호, usage 페이지/누적 비용을 검증했다.
- 전체 Go 테스트·vet, CLI/TUI/resourceclient/lifecycle race, uncached integration,
  payday 생성물 일치 검사를 통과했다. 실제 OAuth/유료 모델 호출은 실행하지 않았다.

## 2026-09-12 — 검정 배경·indicator 여백·명령어 오버레이

- 화면과 입력창/테두리를 검정으로 채우고 첫 두 칸을 indicator 전용으로 정리했다.
  상태바의 sessions 라벨을 제거하고 제목 없는 세션은 Untitled, 짧은 ID는 id:로 구분한다.
- / 입력 시 대화 영역 하단을 임시 덮는 힌트를 추가했다. 방향키/Tab/Enter/Esc를
  지원하며 스크롤·원격 이벤트를 변경하지 않는다. /context·/compact·/usage는
  인자 유무와 무관하게 로컬 Unimplemented를 표시하며 모델 호출은 하지 않는다.
- 메트릭을 어둡게 하고 시간→비용→토큰 순으로 배치했다. 비용만 왼쪽 정렬하고
  나머지는 고정 폭 칸 안에서 우측 정렬/후행 심볼을 쓴다. USD 접미사를 제거하고
  시간은 HH:MM:SS로 표시하며 0인 단위는 더 어둡게 렌더링한다.
- 전체 Go 테스트·vet, CLI/TUI race, PTY 화면 이동/복구와 새 오버레이/무전송/
  들여쓰기/시간 형식 회귀 테스트를 통과했다.

## 2026-09-12 — 대화 집중 레이아웃과 로컬 도움말

- 에이전트 이름/본문에 두 칸 들여쓰기를 복원했다. 메트릭은 답변 뒤 한 줄을 비우고
  왼쪽 정렬하며 16셀 고정 폭과 1k/1.2k/1m 축약 표기로 숫자 변화에 따른 이동을 막는다.
- 상단 정보와 상시 단축키를 없애고 최하단 한 줄 세션 정보로 대화 영역을 확장했다.
  승인/오류 안내는 입력창 위 상태 영역에 유지한다.
- /help는 해당 세션의 로컬 대화 위치에 도움말을 표시한다. 에이전트 전송, 원격
  이벤트 변경, cursor 진행 없이 작동하며 이후 답변이 도움말 다음에 이어진다.
- 고정 폭/축약 경계값, 최하단 배치, 도움말의 무전송/세션 격리 회귀 테스트를 추가했다.
  전체 Go 테스트·vet, CLI/TUI race, PTY 화면 이동/터미널 복구 및 렌더링 검증을 통과했다.

## 2026-09-12 — 대화 정렬과 턴 사용량 요약

- 사용자 입력 시각 색을 어둡게 하고 본문 바깥 여백을 제거해 >를 왼쪽 끝에 맞췄다.
  에이전트 이름만 브랜드색을 유지하며 대답 본문은 밝은 흰색으로 통일했다.
- completed JSON 대신 응답 바로 다음 줄에 토큰/캐시/비용/시간 심볼 요약을
  우측 정렬한다. 좁은 화면에서는 지표 단위로 줄바꿈한다. 실패/중단은 유지한다.
- Claude result usage/cost/duration과 Codex tokenUsage.last를 사용하며 없는 값은
  만들지 않는다. 제공 시간이 없을 때 이벤트 간 경과 시간은 ≈로 구분한다.
- 사용량 추출/누적값 배제/누락값/실패 표시, 한글 너비/정렬 및 본문 색 회귀 테스트를
  추가했다. 원본 이벤트와 API는 변경하지 않았다.
  전체 Go 테스트·vet, CLI/TUI race 및 실제 PTY 이동/복구 회귀 테스트를 통과했다.

## 2026-09-12 — 브랜드 팔레트와 대화 표시 개선

- 지정한 네 브랜드 색과 파스텔 도구/진단 색을 적용했다. Claude 테라코타 및
  Codex 검정 배경/흰 글자 이름 배지와 본문 색으로 발화자를 구분한다.
- 입력창만 좌우 여백을 제거하여 터미널 전체 너비를 사용한다. 본문 여백은 유지한다.
- 사용자 메시지는 로컬 입력 시각, > 첫 줄, 이어지는 줄 들여쓰기로 표시한다.
  state 이벤트는 본문에서 제외하고 최신 조회 상태를 하단에 표시한다. 저널은 보존한다.
- 시간/한글 줄바꿈, 에이전트 스타일/ANSI 차단, 상태 교체/원본 보존,
  40/80/120열 입력창 양끝 정렬 회귀 테스트를 추가했다.
  전체 Go 테스트·vet, CLI/TUI race 및 실제 PTY 이동/터미널 복구 테스트를 통과했다.

## 2026-09-12 — 프로젝트·대화 TUI 디자인 및 입력 개선

- Codex CLI의 대화 중심 구성과 편집 경험을 참고하여 프로젝트 정보/세션 카드,
  대화 역할/도구 출력, 승인 상태, 하단 rounded 입력창/단축키의 시각적 계층을 정리했다.
  밝은/어두운 터미널 대응 강조색과 Unicode 셀 너비 기반 자르기를 적용했다.
- 여러 줄 편집·붙여넣기, Enter 전송, Alt-Enter/Ctrl-J 줄바꿈, Ctrl-X 지우기,
  세션별 메모리 초안 복구를 추가했다. Esc로 초안을 지우지 않으며 PgUp/PgDn은
  본문만 스크롤한다. 기존 생성·로그인·승인·중단·재접속 API 동작은 유지했다.
- 한글/붙여넣기/명시적 전송/초안/크기별 렌더링 회귀 테스트를 추가했다.
  전체 Go 테스트·vet, CLI/TUI race, 실제 PTY 화면 이동/터미널 복구,
  uncached integration 및 diff 검사를 통과했다.
  실제 OAuth나 유료 모델 호출은 이 UI 변경의 검증에 사용하지 않았다.
  참고: https://learn.chatgpt.com/docs/codex/cli (공식 Codex CLI 문서).

## 2026-09-12 — 워크스페이스 진입 명령과 프로젝트/세션 화면

- install/uninstall을 최상위로 되돌리고 up/down/it을 추가했다. up은 Account 없이
  프로젝트만 준비해 대시보드를 열며, 실행 중인 기존 프로젝트는 재준비 없이 연결한다.
  it은 해당 프로젝트의 최신 생성 세션 화면으로 연결하고 없으면 생성 없이 안내한다.
- 프로젝트 화면에 정보·세션 목록, 방향키/Enter 선택, n 새 세션, s 중지, d 삭제 확인을
  추가했다. 생성 중에는 터미널을 반환해 Account 검색·공식 로그인 후 세션 화면으로
  이어진다. Ctrl-Q는 프로젝트 화면으로 돌아가며 Ctrl-C는 연결만 끊는다.
- payday Project/Session.Erase를 lifecycle 규칙에 맞게 열었다. 프로젝트 삭제는 소유
  컨테이너 정리 후 프로젝트와 세션을 목록에서 제외한다. 세션 삭제는 실행 agent 중지를
  확인한 뒤 제외한다. 소스/볼륨/저널을 보존하고 Listed tombstone은 일반 조회·제어와
  snapshot 재등장을 막는다. 같은 workspace 재등록 시에도 이전 세션은 제외한다.
- 기존 프로젝트당 실행 세션 하나 제약, 저수준 project down의 등록/세션 보존 동작은
  유지한다. DB 재구축은 tombstone을 잃을 수 있으므로 resource DB 백업이 필요하다.
- 준비 반복 시 세션·Account 부작용 없음, 프로젝트 격리·최신 선택·삭제 확인 대상 고정,
  Ctrl-Q·PTY 터미널 복구, 실제 RPC live-session 삭제와 서버 재시작 후 제외를 검증했다.
- 전체 Go 테스트·vet, CLI/TUI/lifecycle/integration race, diff 검사 통과.
  Docker runtime probe 및 실제 OAuth/유료 모델 호출은 이번 변경에서 실행하지 않았다.

## 2026-09-12 — 리소스 중심 CLI와 기본 표 출력

- 공개 작업 명령을 project/session/account/manager/config 및 backend/binding 하위로
  재편했다. projects → project ls, ls → session ls, new → session new,
  up/recreate/down → project 하위, 설치·서버 실행·업데이트 → manager 하위로 옮겼다.
  로그는 manager logs / project logs PROJECT로 분리하며 config show를 명시한다.
  이전 최상위 명령의 호환 별칭은 제공하지 않는다. 내부 프로세스 진입점은 유지한다.
- 데이터 명령의 기본 출력은 표이며 --format table|json을 추가했다. 명령별 옵션이
  루트 옵션보다 우선한다. 목록 주요 열, 상세 FIELD/VALUE, 빈 목록, 한글 표시 폭,
  터미널 너비에 따른 축약과 제어문자 비활성화를 처리한다. JSON/JSONL은 기존
  직렬화 필드·큰 정수·줄바꿈을 보존한다. raw exec/shell/logs/login/TUI는 변환하지 않는다.
- account status도 포맷을 지원하고 파일 존재와 vendor 인증 검증 여부를 구분한다.
  Docker manager의 시작 인자·이미지 CMD, 도움말·완성·안내문, 현재 문서와 probe 호출을
  갱신했다. 자동화 probe는 JSON을 명시하고 과거 릴리스 문서는 원래 버전의 설명을 유지한다.
- 전체 Go 테스트·vet, CLI/installer/integration race, diff 및 probe JS 구문 검사 통과.
  Bake --print 설정 검증 통과. 이번 변경에서 Docker runtime probe나 실제 OAuth는 실행하지 않았다.

## 2026-09-12 — Claude 로그인 코드 입력 표시

- 대화형 Claude auth login을 파이프로 연결하고 cxz 입력 표시기를 추가했다.
  빈 상태는 `[     ]`, 입력된 상태는 길이와 무관하게 고정된
  `[ *** ] inserted; ctrl+x to clear.`를 표시한다. 코드 자체와 글자별 마스킹은 출력하지 않는다.
- Ctrl-X는 미제출 입력을 지우고 Enter는 공식 CLI에 한 줄로 전달한다. 제출/취소/종료 시
  입력 모델을 초기화하며 Esc/Ctrl-C 종료 시 자식 프로세스와 터미널을 정리한다.
  OAuth URL·코드 교환·인증 저장은 기존 공식 CLI와 격리된 staging 경로를 유지한다.
- 고정 Claude 2.1.267의 readline 기반 수동 코드 처리를 로컬 설치 바이너리에서 확인했다.
  입력 모델·합성 자식 프로세스·Linux PTY에서 비노출 표시, 지우기, 제출, 취소,
  종료와 터미널 복구를 검증했다. 실제 OAuth 인증은 실행하지 않았다.
- 전체 Go 테스트·vet, accounts/CLI race, diff 검사 통과.

## 2026-09-12 — 프로젝트 첫 로그인을 up 흐름에 연결

- project-local-oauth의 인증 파일 누락을 구조화된 gRPC ErrorInfo로 전달한다.
  대화형 CLI new/up/recreate는 준비된 프로젝트의 기존 공식 로그인 워크플로를
  실행하고 성공하면 세션 생성/재접속을 한 번 재시도한다.
- 번호/문자열 오류 파싱에 의존하지 않는다. 중앙 backend, 다른 계정, 인증 파일 손상은
  자동 로그인 대상에서 제외한다. 계정 binding·소유권·활성 세션 검사와 OAuth 격리는 유지한다.
- 재시도 시 recreate는 해제하고 request ID와 새 세션 여부는 보존한다. 로그인 출력은
  stderr로 분리한다. 취소 시 준비된 프로젝트를 보존하며 비대화형은 로그인 명령을 안내한다.
- CLI 성공/실패/취소/재시도 제한 테스트와 실제 RPC의 누락 인증 details 전달,
  인증 전 세션 미생성 및 합성 인증 설치 후 동일 요청 성공 통합 검증을 추가했다.
  실제 OAuth 브라우저 로그인이나 유료 모델 호출은 실행하지 않았다.
- 전체 Go 테스트·vet, CLI/accounts/server/integration race, diff 검사 통과.

## 2026-09-12 — 검색 가능한 계정 선택 TUI

- CLI new/up/recreate의 정수 전용 Fscanln 선택기를 Bubble Tea 화면으로 교체했다.
  이름을 입력하면 파싱이 실패하고 나머지 문자가 셸에 남을 수 있던 경로를 제거했다.
- 방향키 선택, 하단 상시 검색 입력, 번호·display name·alias 정확 일치 및
  이름·alias·agent 부분 검색을 제공한다. 중복 후보, 검색 결과 없음, 긴 목록 스크롤,
  한글 입력과 화면 크기 변경을 처리한다. Esc/Ctrl-C는 계정 선택 없이 취소한다.
- 입력/출력은 CLI의 stdin/stderr를 사용해 stdout JSON과 분리한다. 비대화형 계정
  명시 및 기존 세션의 Account 유지 동작은 변경하지 않았다.
- 모델/실제 입력 디코더 회귀 테스트 및 Linux PTY에서 main 입력·Ctrl-C 취소,
  터미널 상태 복구와 입력 잔여 없음 검증을 추가했다.
- 전체 Go 테스트·vet, CLI/TUI race, diff 검사 통과. Docker나 실제 인증 상태는 변경하지 않았다.

## 2026-09-12 — 전체 CLI 입력 계약 점검

- `account add AGENT ACCOUNT`, `update IMAGE`, 내부 `_new-local ACCOUNT WORKSPACE`
  형식으로 필수값을 positional에 드러냈다. agent에는 기본값 없이 enum/completion을 제공한다.
- `project set` 변경값 누락, 계정/프로젝트 alias, 모델/이름, 빈 문자열, config 키,
  reply JSON을 설정 IO·연결 전에 검사한다. 비대화형 new의 Account 누락도 즉시 거부한다.
- 재접속/대화형 선택에 필요한 선택 옵션은 유지하고 도움말에 조건·실제 기본값을 명시했다.
  중앙 인증에 적용되지 않는 `--project`는 계정 조회 후 명시적으로 거부한다.
- 사용되지 않는 루트 agent/claude-config 옵션과 항상 실패하던 login 진입점을 제거했다.
  Account가 agent를 결정하므로 `config set agent`는 거부하고 기존 설정의 unset은 허용한다.
- 모든 하위 명령 도움말, agent completion, Account Add RPC 전달, 잘못된 입력의
  state 미생성·무연결 회귀 테스트를 추가했다. 예제와 Docker probe의 CLI 호출도 갱신했다.
- 전체 Go 테스트·vet, CLI/integration race, diff 검사 통과. 이번 변경에서 Docker probe나
  실제 OAuth/유료 모델 호출은 실행하지 않았다. [전체 명령 점검표](cli.md).

## 2026-09-12 — 세션 조건 fail-fast

- `up`이 컨테이너를 준비한 뒤 빈 AccountRef로 새 세션 binding을 만들던 경로를 수정했다.
- 공통 resource client와 CLI가 등록·provision/recreate 전에 계정·agent/backend,
  활성 세션 충돌·모델 불변 조건을 확인한다. 조회 실패는 즉시 반환한다.
- 기존 세션의 계정을 유지하며, 첫 up/recreate에서는 new와 같은 Account 선택을
  제공한다. 비대화형 계정 누락은 `--account` 안내로 실패하고 준비 메시지도 출력하지 않는다.
- 새 세션 조건 실패 시 Project Add/Up/Recreate가 호출되지 않는 테스트와 CLI 회귀,
  기존 세션의 모든 재접속 상태 및 로그인 prepare-only 예외 테스트를 추가했다.
- 전체 Go 테스트·vet, resourceclient/CLI/integration race, diff 검사 통과.

## 2026-09-12 — devcontainer 신뢰 검사 진단

- `--trust-config` 요구 오류에 설정 파일명과 차단된 항목의 JSON Pointer 경로,
  탐지 이유를 모두 표시한다. devcontainer JSONC와 Compose YAML 양쪽에 적용했다.
- 기존 차단 기준은 변경하지 않았다. 출력 순서를 고정하고 경로의 특수문자를
  이스케이프하며 명령·환경변수·mount 원문 값은 출력하지 않는다.
- 각 탐지 규칙, 여러 원인 동시 표시, 값 비노출, 경로 이스케이프, explicit trust,
  recreate 전 preflight의 파일명·항목 경로를 회귀 테스트에 추가했다.
- 전체 Go 테스트, workspace race·vet, diff 검사 통과.

## 2026-09-12 — Codex 중앙 인증 공급

- 신규 Codex Account의 기본 backend를 `brokered-access-token`으로 변경했다.
  기존 Account의 backend는 유지하고 Claude의 프로젝트별 OAuth도 유지한다.
- 공식 Codex device login과 app-server managed refresh를 중앙 Account별 격리
  프로필에서 실행한다. cxz는 OAuth endpoint/code 교환을 재구현하지 않는다.
- 프로젝트별 capability를 비공개로 전달하고 Unix socket을 통해 access token만
  공급한다. AuthBinding은 프로젝트별 공급 권한을 고정하며 token/capability는
  공개 resource·audit·대화 journal에 넣지 않는다. 프로젝트는 ephemeral 인증을 쓴다.
- 재로그인 시 workspace/account ID와 사용자 subject를 고정하고, 중앙 로그인·갱신은
  계정 lock으로 직렬화한다. 다른 계정, 위조 capability, manager 중단은 명시적으로 실패한다.
- 전체 Go/race·vet·payday 코드 생성 일치·diff 검사 통과. Docker에서 중앙 로그인으로 두 프로젝트 사용, 동시 갱신,
  개인/회사 프로필, 승인·중단, resume·manager restart·project recreate 통과.
  non-root(node 사용자) 프로젝트를 포함한 최종 회귀도 통과했다.
- 네이티브 Codex 0.154.0도 합성 JWT로 external login 수용·account/read·ephemeral
  auth.json 비저장을 확인했다. 실제 OAuth 로그인/갱신과 유료 모델 호출은 하지 않았다.
- [인증 설계와 제약](accounts.md): 공식 external-token API는 실험적이며, 토큰 추출은
  고정 버전의 managed auth.json 형식에 의존한다. 사용자 인증/roster와 권한 철회 UI는 별도다.
- 추가 검증 중 공유 Docker 주소 풀 고갈을 만났지만 이번 시험의 이전 소유 네트워크만
  정리하고 재시도하여 통과했다. 타 프로젝트 자원/prune은 사용하지 않았다.
- [검증 요약](../testdata/recovery/codex-broker/summary.json).
- 시험에 사용한 두 manager와 프로젝트 컨테이너, 합성 인증 state/tools 볼륨 및
  소유 네트워크는 삭제했다. scratch workspace·이미지 빌드 캐시는 보존했다.

## 2026-09-12 — AuthBackend / AuthBinding 분리

- AgentKind별 기본/지원 backend factory registry와 `Info/Binding/Login/Check/Launch`
  계약을 도입했다. 기존 프로젝트 OAuth 로그인·인증 확인·환경/인자 구성을 backend로 옮겼다.
- payday Account.auth_backend, AuthBinding(domain 10), Session.auth_binding을 선언·생성했다.
  binding은 project/account/backend별로 안정적인 ID를 가지며 재시도해도 중복 생성되지 않는다.
- CLI `account backends`, `account bindings`, `account add --auth-backend`를 추가했다.
  실제 login/status/new/resume과 manager→project 경로가 backend/binding을 사용한다.
- backend/binding은 manifest와 조회 JSON에 보존한다. 누락·불명 backend와 다른 범위의
  binding은 실행/복구 시 거부한다. 기본값은 Account 생성 시에만 선택한다.
- Claude/Codex의 프로젝트별 OAuth만 활성화했다. 중앙 토큰 공급·API key·실계정 인증은
  추가하지 않았다. 실제 OAuth 로그인·유료 모델 호출 없이 합성 agent로 검증했다.
- 전체 Go 테스트·race·vet와 payday 코드 생성 일치 검사를 통과했다. Claude/Codex
  로그인 인자·환경 구성, 실패한 로그인 시 기존 인증 보존, 잘못된 저장 binding의
  실행 전 거부를 테스트했다.
- Docker `owned-accounts.mjs`: 지원 backend 조회·미지원 backend 거부, 재로그인 시
  binding 중복 방지, 계정/프로젝트별 binding 분리, Session 전달, manager 재시작 및
  project recreate 후 동일 backend/binding 유지 통과. 기존 Account 격리 회귀도 통과했다.
- `cxz-container-recreate.mjs`: non-root·network-none에서 대화 보존, 명시적 resume과
  새 Run, 과거 승인 거부, 진행 중 컨테이너 소실 후 대화 복구 통과.
- 합성 인증 시험에 사용한 전용 컨테이너와 state/tools 볼륨은 정리했다.
  scratch workspace·이미지 캐시는 보존했다. [검증 요약](../testdata/recovery/auth-backends/summary.json).

## 2026-09-12 — Account 인증 프로필

- payday Account 리소스(domain 9), 고정 Session.account 연결, 계정 등록/조회,
  프로젝트별 로그인·상태 확인, CLI --account와 TUI 계정 선택을 구현했다.
- 최초 구현의 중앙 vault 복제는 architecture §4.2와 충돌하여 최종 구현에서 제거했다.
  실제 OAuth grant는 프로젝트×Account별로 독립 발급·저장한다. 계정별 HOME/config를
  사용하고 inherited vendor 인증 환경변수를 차단한다. refresh token을 복제하지 않는다.
- 누락된 인증·다른 vendor·다른 Account로의 live attach를 거부한다. 정지 세션의
  resume은 계정을 유지하며, 재로그인은 다음 실행 시 반영한다.
- 전체 Go 테스트 및 `CXZ_TEST_RACE=1 go test -race ./... -count=1` 통과.
  Account 리소스/인증 환경/갱신 보존/DB 복구 테스트를 추가했다.
- Docker `owned-accounts.mjs`: 실제 CLI의 staged login/status(합성 vendor), 두 Account
  별도 세션, 계정별 HOME/config, 환경 인증 차단, 잘못된 live attach/활성 로그인 거부,
  원래 계정 resume, manager 재시작·project recreate 후 동일 계정 유지 통과.
  manager에 인증 저장소가 생기지 않는 것도 확인했다.
- 별도 Project에서 동일 Account를 선택해도 기존 프로젝트 인증을 사용하지 못하고
  독립 로그인을 요구하는 것을 확인했다. 기존 이름/alias up·exec·logs·rename도 재검증했다.
- 최종 `go vet ./...`, `go tool pd gen --check .`, `git diff --check` 통과.
- `cxz-container-recreate.mjs`: non-root·network-none에서 Account/대화 보존,
  새 Run·과거 승인 거부·진행 중 컨테이너 소실 후 복구 통과.
- 실계정 로그인/유료 대화는 수행하지 않았다. `owned-session-live.mjs`는 명시적으로
  인증된 Project/Account를 받도록 바꾸고, host token 복사 경로를 제거했다.
- 합성 인증을 사용한 시험 manager/project 컨테이너와 전용 state/tools 볼륨은 정리했다.
  실제 계정에는 로그인하지 않았으며 scratch workspace와 이미지 빌드 캐시는 보존했다.
- 범위·제약: [Account 설계](accounts.md). 사용자 인증/roster와 프로젝트별 허용 계정
  정책은 별도이며, 동일 OS 사용자 간의 적대적 코드 격리를 주장하지 않는다.

## 2026-09-12 — Project display name / alias

- payday Project의 `name`을 표시 이름으로 사용하고 `alias = 4`에 전역 unique
  인덱스를 선언했다. 생성된 ProjectRef의 alias 조회와 metadata-only Patch를 지원한다.
- cld의 짧은 이름/단어 이니셜/긴 단어 절단 방식을 참고해 자동 alias를 생성한다.
  자동 충돌은 ID 기반 suffix로 해소하고 기존 alias는 바꾸지 않는다. 직접 지정한
  중복/잘못된 alias는 거부한다. Patch는 version 조건을 요구하고 상태 수정은 막는다.
- `project add --name/--alias`, `project set --name/--alias`, new/up/recreate의
  이름 옵션을 추가했다. up/down/attach/exec/shell/login/logs가 공통 project resolver를
  사용한다. ID·경로 → alias → 표시 이름 순서이며 중복 표시 이름은 임의 선택하지 않는다.
- JSON 조회와 TUI에 표시 이름·alias를 노출하고 프로젝트 인자 자동완성을 추가했다.
  표시 이름만 바꿔도 alias는 유지하며, alias를 바꾸면 이전 alias는 더 이상 해석하지 않는다.
- 통과: 자동 생성/충돌, 명시적 중복 거부, generated alias ref, 변경 후 ID 유지,
  재시작 후 유지, 등록 재시도 시 metadata 보존, 상태 Patch 우회 차단, 전체 race.
  Docker alias 흐름도 `scripts/probes/owned-project-names.mjs`로 통과했다:
  alias 기반 up/exec/logs/down, 이름·alias 변경 후 동일 session 재접속.
  테스트 manager/container는 정리하고 작업 파일·볼륨은 보존했다.

## 2026-09-11 — Docker Bake 및 edge 게시

- cld의 build → app 흐름을 참고해 루트 `Dockerfile`, `docker-bake.hcl`,
  source-only `.dockerignore`를 추가했다. build는 amd64/arm64 정적 바이너리를
  `dist/linux-*/cxz`로 내보내고 app은 기존 설치용 Dockerfile로 이미지를 만든다.
- 설치 명령의 tar context도 같은 아키텍처별 경로를 사용한다. release workflow의
  임시 Dockerfile 생성/sed를 제거하고 같은 Bake app target을 사용한다.
- CI는 test 성공 후 이미지를 빌드한다. main push만 GHCR에 `edge`와
  `sha-<commit>`을 게시하고, PR은 로그인/게시하지 않는다. 이전 main 실행이 늦게
  edge를 덮어쓰지 않도록 workflow concurrency를 설정했다.
- 이미지 라벨과 바이너리 version 출력에 commit 정보를 포함한다.
- 로컬 확인: Bake 설정 해석, amd64/arm64 정적 바이너리 빌드, workflow actionlint,
  전체 Go 테스트 통과. amd64 Bake 이미지의 `version` 실행에서 edge 및 build hash를
  확인했다. CI는 별도로 두 아키텍처 이미지를 빌드하고 main에서 게시한다.

## 2026-09-11 — proto 버전 표기 제거

- 사용자 요청에 따라 공개 proto 패키지를 `cxz`, 정의 경로를 `proto/cxz`,
  확장 경로를 `proto/ext/cxz`로 정리했다. 버전 접미사나 하위호환 별칭은 없다.
- 내부 view model도 버전 대신 `cxz.runtime` namespace와
  `internal/runtimeproto` 경로를 사용한다. 공개 서비스로 등록하지 않는다.
- payday/ORM 및 내부 protobuf 생성 코드를 새 이름으로 재생성했다.
- 전체 Go 테스트와 proto namespace 회귀 테스트를 통과했다.

## 2026-09-11 — payday resource framework 전환

이전의 config/DB/grpcx만 이용한 구현은 사용자가 요청한 payday 프레임워크
사용을 충족하지 않았다. 아래 전환은 기존 rc.1 태그와 별개의 변경이다.

- `proto/cxz/{project,session}.proto`에 tenant 없는 global 리소스를 선언하고
  `proto/ext/cxz/*_svc.ext.proto`에 Up/Down/Recreate 및
  Resume/Send/Reply/Interrupt/Stop/Events/History를 선언했다.
- 버전 고정 Go tools와 Buf lock으로 실제 `pd gen`을 실행했다.
  `resource/`, `server/bare/`, `server/pd/`, `internal/ent/`가 생성 산출물이다.
- `server/lifecycle/`가 generated Sink·ent SQLite·payday audit/gate·Watch를
  실제 서버 요청 경로에 연결한다. Docker/process 작업은 DB transaction 밖에서
  실행하고, 내부 projection 쓰기도 commit 이후 Watch 알림을 발행한다.
- CLI/TUI 및 manager→project는 generated resource client를 사용한다.
  기존 Sessions 공개 service 등록은 제거했다. runtime DTO만 내부에 남겼다.
- Project.Up은 provisioning만 한다. Session.Add와 Resume는 별도 요청이다.
  runtime/vendor ID·저널·생성 재시도 키를 유지하면서 payday domain UUID에 매핑한다.
- 통과: resource API를 통한 생명주기/승인/중단/복구 및 두 SQLite DB 재구성,
  xli CLI 전달, generated audit 기록·tenant 불필요·stale run 거부,
  상태 Patch 우회 거부, payday Watch snapshot/update, `pd gen --check`.
- 통과: `go test ./...`, 전체 `CXZ_TEST_RACE=1 go test -race ./... -count=1`,
  `go vet ./...`, `pd gen --check`, Docker foreign/recreate/프로젝트 권한/단일 agent/down
  경계, manager 강제 종료 checkpoint 복구와 동시 up 수렴.
- 통과: non-root 컨테이너 재생성 후 대화·저널·vendor ID 유지, 새 run의 명시적 Resume,
  stale 승인 거부, 진행 중 turn 손실 후 재접속. 인증 없는 deterministic agent로 검증했다.
  증거: `testdata/recovery/payday-resource/summary.json`.
- 첫 Docker 테스트는 개발 바이너리를 CGO 기본값으로 빌드해 Alpine loader 실패;
  배포와 동일한 CGO_ENABLED=0 정적 빌드로 재검증했다. 실제 vendor 인증/과금 대화는
  이번 전환에서 다시 실행하지 않았고, 이전 vendor-specific 잔여 검증은 그대로 남는다.
- 사용자 인증/roster, HTTP/Web UI는 이번 범위에 추가하지 않는다.
  Dockerfile은 기존 `internal/installer/image.Dockerfile`; Bake 전환은 하지 않았다.
- 전체 state volume 백업은 필요하며 resource 표시 이름/설명과
  audit는 저널로 재생성되지 않는다.

설계: [payday resource API](plans/payday-resources.md).

## 2026-09-11 — 실사용 보강·prerelease 게시 (남은 검증 별도)

순서: 복구·재시도 → TUI 설정/진단 → 설치·배포. 웹 UI/사용자 인증은 후속 범위.

- 설치 identity를 리소스 생성 전에 원자적으로 보존해 실패한 첫 설치도 같은
  owner/volume을 재사용한다. 동일 이미지의 설치 재호출은 readiness 재확인,
  정지한 manager는 시작한다. endpoint는 기존 manager 제거 전에 검증한다.
- 프로젝트 준비 단계/시도 횟수를 manifest에 기록한다. manager 재시작은 실행 중
  job을 interrupted로 표시하며 SQLite 재구성 후에도 마지막 단계를 유지한다.
- 시작/프로젝트 조회 시 소유 label 기반 inventory를 재확인한다. 생성 후 저장 전에
  끊긴 container ID를 복구하고, 중복 소유 컨테이너는 임의 선택하지 않는다.
- 재시도는 실제 리소스 소유권을 다시 확인하고 수렴한다. devcontainer 사용자 hook은
  다시 실행될 수 있어 멱등하게 작성해야 한다. prompt 자동 재전송은 하지 않는다.
- 통과: 설치 실패 identity 재사용, endpoint 사전 거부, checkpoint/DB 재구성,
  inventory 복구·foreign/중복 거부, 기존 workspace/CLI 테스트.
- Claude 반복 복구 미통과는 아직 해소하지 않았으며 후속 검증 항목으로 유지한다.

추가 구현:

- `config`의 기본 agent/vendor별 model, `new --model`, TUI 새 세션 기본값.
  model은 세션 manifest/API에 저장하고 Claude argv/Codex thread start/resume로 전달한다.
  기존 세션의 모델은 변경하지 않는다. 설정은 secret/권한 변경을 허용하지 않는다.
- TUI 진단/실패 payload와 인증 의심 메시지의 login 안내, `doctor`, `logs`, `version`.
- 버전 고정 `update --image`, 이전 manager 이미지 `rollback`. 이미지 다운로드/플랫폼
  검증을 기존 manager 제거 전에 수행한다. named volumes/project 프로세스는 보존한다.
- Linux amd64/arm64 release build와 checksum, SHA-pinned CI/release workflow.
  원격 게시/arm64 실실행은 로컬 cross-build와 별도로 검증해야 한다.
- 통과: 전체 test/race/vet, 설정 offline 검증·0600·잘못된 값/secret key 거부,
  API model 전달·영속 세션·Codex protocol, TUI 진단/로그인 안내.
- 실제 Docker foreign/명시적 recreate/RO tools/내부 범위/down 보존 경계 6개 통과.
- Claude 재검증에 쓸 기존 access token의 유효기간이 부족하다. refresh token을 사용하거나
  사용자의 계정을 조작하지 않았고 반복 복구의 기존 미통과 판정은 유지한다.
- 운영 절차와 범위는 [operations.md](operations.md). 준비 중 강제 종료/동시 재시도,
  update/rollback, Codex live 및 release 게시 검증은 아래에 결과를 추가할 예정이다.

실제 검증 추가:

- `owned-provision-retry.mjs`: hook 실행 중 manager SIGKILL → interrupted checkpoint,
  저장 직전 container ID 복구 → 동시 up 2회에서 동일 container/session 유지, 로그 조회 통과.
  최초 probe의 short/full Docker ID 비교 오류는 `--no-trunc`로 수정 후 전체 재실행했다.
- `owned-install-update.mjs`: 빈 state 설치, 반복 install, 정지 manager 시작, 존재하지 않는
  update 이미지 거부 시 기존 manager/locator 보존, update/rollback identity·volume 유지 통과.
  별도 검증 manager는 제거했고 named volumes와 locator는 보존했다.
- Codex 0.154.0 실제 인증 대화, manager restart 동일 run, 도구 승인/거절,
  실행 중 interrupt, 컨테이너 재생성 2회 후 동일 vendor ID/대화 기억: 7개 모두 통과.
  이번 검증의 일회성 access token은 제거했고 refresh token은 복사하지 않았다.
- Linux amd64/arm64 artifact cross-build 및 SHA256SUMS 검사 통과.
  Claude 재인증/반복 복구는 미통과로 남겨 prerelease와 안정판 완료를 구분한다.

배포/인계 결과:

- `v0.1.0-rc.1`을 GitHub Release에 게시했다. release commit은 `4a386ad`이며
  xli CLI/설정/진단/복구/업데이트 기능을 포함한다. 이후 main 변경은 검증 도구와 문서다.
- GitHub CI 및 Release workflow 성공. 공개 바이너리 두 아키텍처를 다시 다운로드하여
  SHA256 검증, amd64 version/revision 일치를 확인했다.
- Docker 로그인 설정이 없는 별도 client에서 GHCR digest로 빈 설치 → doctor →
  프로젝트 foreign/recreate/내부 범위/RO tools/down 6개 경계 검증 통과.
- 실행 중 Codex supervisor를 둔 manager update/rollback에서도 같은 run 유지.
  강제 컨테이너 제거 및 manager SQLite 재구성, 실제 TUI 자동 재접속도 재검증 통과.
- Claude 문제를 분리하는 `CXZ_PROBE_MEMORY_ONLY=1` 모드를 추가했다. syntax 검증만 했으며
  Claude 실제 최소 재현은 재로그인 후 실행해야 한다. 안전장치는 변경하지 않았다.
- arm64는 artifact/image 빌드와 checksum만 검증했다. 현재 amd64 Docker engine에서 실행은
  `exec format error`로 불가능했다. 공유 엔진에 전역 QEMU/binfmt 설정을 설치하지 않았다.
- 검증용 manager 3개와 이번 프로젝트 컨테이너를 제거했다. 사용자 작업 폴더/영속 볼륨,
  복구 가능한 SQLite backup, 다운로드 산출물은 보존했으며 prune은 실행하지 않았다.
- 정제된 증거: `testdata/recovery/operations-summary.json`.
  최초 vendor 로그인, Claude 반복 복구 재검증, native arm64 인수 검증은 남은 항목이다.
  웹 UI/roster/IDE/외부 백업은 이번 세 우선순위의 범위가 아니며 여전히 후속 계획이다.

## 2026-09-11 — xli CLI 전환

- `flag.FlagSet`, 수동 최상위 명령 분기, `optionsFirst` 순서 보정 제거.
  `xli.Command` 트리, typed flags/args, 명령별 handler로 교체했다.
- payday가 이미 요구하는 xli revision `bf8cac633057`을 직접 의존성으로 사용하며
  payday 버전과 서버/supervisor/agent API는 변경하지 않았다.
- API 연결은 실행 handler에서만 수행한다. 도움말·완성은 설치/네트워크 연결 없이 동작.
- 자동 도움말, zsh completion, vendor/승인 값 및 workspace/config 경로 완성을 추가.
- 명령 IO는 xli의 입출력을 사용한다. JSON stdout과 진행/확인 stderr를 분리하고
  `exec PROJECT -- COMMAND...`의 나머지 인자를 그대로 보존한다.
- 문법 변경: `new --agent codex .`, `recreate --yes .`, `login --agent codex PROJECT`.
  위치 인자 뒤 플래그는 xli 규칙에 따라 거부한다. README와 기존 probe를 갱신했다.
- 내부 `_boot/_project/_supervise/_guard/_bridge/_ready/_login/_new-local`과
  `attach/it`, `tui/watch` alias는 유지한다. 기존 root-level serve flags도 호환 유지.
- 통과: xlitest 도움말/입력 검증/완성/exec 인자, 실제 gRPC 전달 테스트,
  기존 프로세스 통합 테스트, Docker install/경계 probe 6개 검사,
  `CXZ_TEST_RACE=1 go test -race ./... -count=1`, `go vet ./...`.
  vendor adapter를 변경하지 않았으므로 유료 모델 호출/로그인은 재실행하지 않았다.
- 검증 후 이번 실행의 project container와 manager를 제거했다. 기존 workspace,
  named volumes와 private evidence는 보존했고 공유 엔진에 prune을 실행하지 않았다.

## 2026-09-11 — cld 사용 흐름 + Codex 확장 (현재)

이전 로컬 TUI 구현은 기반 단계였다. 현재 완료 기준은
[cld-parity 계획](plans/cld-parity.md)이며, 아래의 과거 제외 범위를 대체한다.

- 구현: Docker manager install/reinstall/uninstall, official devcontainer CLI
  provisioning, image/Dockerfile/Compose 설정, 프로젝트별 state volume와 RO tools.
- 구현: `up/new` 자동 TUI 연결, Claude/Codex 선택, `attach/it`, `login`, `shell/exec`,
  `down/up/recreate`. 외부 컨테이너는 편입하지 않으며 확인한 recreate만 허용.
- 구현: Codex app-server 초기화/thread/turn/approval/question/interrupt/resume.
  공식 schema/docs를 확인해 numeric request ID 보존과 빈 thread 재시작을 처리.
- 실검증 통과: image+기존 hook, Alpine Claude 시작, Compose Codex 시작,
  sidecar DNS, tools readonly, 프로젝트 내부 scoped client.
- 실검증 통과: 실제 Codex 인증 대화, manager restart 시 동일 supervisor run,
  도구 승인/거절, 실행 중 중단, 컨테이너 down/up **2회** 후 동일 vendor thread
  및 대화 기억 복구. `owned-session-live.mjs`의 7개 검사 모두 통과.
  호스트 refresh token은 복사하지 않았고 일회성 access-token 투영은 검증 후 제거.
- 실검증 통과: Claude 대화/승인/거절/중단, manager restart, 첫 컨테이너
  재생성 후 동일 vendor 대화와 기억 복구. 두 번째 재생성의 기억 확인은
  vendor API safeguard refusal(`reasoning_extraction`)로 재현되어 **미통과**로 기록.
  vendor ID와 실패 이벤트는 보존되며, vendor의 안전 설정을 끄지 않았다.
- 실검증 통과: 생성한 기본 non-root 설정, Dockerfile+worker 사용자+hooks,
  공식 git feature, foreign 무변경 거부/명시적 recreate/내부 범위 제한 등 6개 경계 검사.
- 실검증 통과: 실제 TUI Codex attach, Ctrl-N/Tab agent 선택, manager restart
  중 cursor 재접속, Ctrl-C detach. Feature provisioning 중에도 다른 프로젝트를 조회.
- 실검증 통과: Codex 컨테이너 강제 제거 후 같은 세션/thread를 새 run으로 복구;
  manager SQLite 파일을 백업 위치로 이동 후 manifest 재구성, 기존 project run 유지.
  데이터베이스 원본은 named volume에 복구 가능한 이름으로 보존했다.
- 검증 요약: `testdata/recovery/owned-workspaces-summary.json`. 재실행 도구는
  `owned-session-live.mjs`, `owned-boundaries.mjs`, `owned-recovery.mjs`.
- 보강: 준비 확인을 socket 파일 존재가 아닌 실제 RPC로 변경, 프로젝트별
  lifecycle lock으로 다른 프로젝트의 provisioning이 세션 제어를 막지 않도록 함.
- 최종 회귀 통과: `go test ./...`, `CXZ_TEST_RACE=1 go test -race ./internal/... -count=1`,
  `go vet ./...`, protobuf 재생성 후 생성물 diff 없음, `git diff --check`.
- 중간 커밋: `bab4541` (manager/Codex), `e9fc20e` (CLI/Compose/복구 보강).
- 정리: 검증 소유 label을 가진 프로젝트 컨테이너와 manager를 제거했다.
  테스트 workspace, named volumes, DB backup과 private journal은 보존했다.
  공유 Docker 엔진의 다른 프로젝트나 이미지/볼륨을 prune하지 않았다.

현재 인계 범위: cld식 설치→devcontainer 생성→vendor 선택→TUI 접속/제어→복구의
핵심 흐름 구현. 전체 cld 편의 기능의 복제나 웹 MVP 완료를 뜻하지 않는다.
최초 vendor 로그인, 웹/IDE UI, roster 인증, 중앙 로그인 broker, 자동 release update,
dotfile/SSH forwarding, off-host backup은 미구현/사용자 단계로 명시한다.
Claude의 두 번째 반복 복구 검증은 위 vendor 응답 제한 때문에 미통과로 유지한다.

## 2026-09-11 — 로컬 TUI 기반 구현 (이전 기록)

요청 범위: payday + SQLite, TUI의 세션 생성·대화·승인·중단·재접속.
사용자 인증·브라우저 UI는 이번 구현에서 제외한다. 서버는 다른 OS 사용자가
접근할 수 없는 Unix socket만 제공한다. Claude 인증은 기존 로컬 CLI 설정을 사용한다.

- 완료: Claude stream-json 실측, 승인/질문/interrupt/resume 및 컨테이너 재생성 probe.
  증거: `testdata/protocol/claude/manifest.json`, `docs/plans/claude-code-verification.md`.
- 완료: protobuf API, payday 런타임, SQLite 세션 레지스트리/이벤트 projection,
  독립 supervisor, TUI/CLI 기본 구현.
- 통과: `TestLifecycle` — 실제 daemon/supervisor 프로세스 + deterministic agent.
  생성 멱등성, 작업공간 중복 거부, 대화, 승인/거절/질문, 중단, cursor 재생,
  daemon SIGKILL 중 출력 보존, supervisor SIGKILL 후 새 run으로 resume,
  이전 run 승인 거부, SQLite 재구성.
- 통과: 저널 torn-tail 복구 및 committed corruption 거부 단위 테스트.
- 통과: 실제 Claude Code 2.1.267 live integration 7개 경로 — 대화, 승인,
  거절, 질문, 실행 중 중단, daemon 재접속, 동일 vendor 세션 resume.
  중단된 명령의 예정 종료 시각 이후에도 파일 부작용이 없음을 확인했다.
  정제된 결과: `testdata/recovery/claude-live-summary.json`.
- 통과: 실제 터미널에서 TUI 세션 생성, 대화, F2 승인, F3 거절, 질문 JSON 응답,
  F4 중단, Ctrl+C detach 후 살아 있는 세션 확인, Ctrl+R resume,
  승인 대기 중 daemon SIGKILL 후 TUI 자동 재접속 및 승인 처리.
  키 매핑·중복 이벤트 제거·
  터미널 escape 제거는 TUI 단위 테스트로도 검증한다.
- 통과: cxz 앱 자체의 비루트 Docker 재생성 4개 검사. 승인 대기 및 실행 중
  컨테이너 소실 후 영속 volume으로 registry/journal 복구, 새 run resume,
  stale 승인 거부, 복구 후 대화. fixture agent 사용, 네트워크 없음.
  결과: `testdata/recovery/cxz-container/summary.json`. 공유 엔진의 테스트 소유
  컨테이너/volume만 label 확인 후 제거했다.
- 보강: raw와 정규화 이벤트를 단일 JSONL batch로 원자적으로 기록하고,
  중간에 끊긴 batch는 전부 제외한다. 기존 단건 형식도 읽는다.
- 보강: supervisor SIGKILL 시 shell 자식까지 종료하는 liveness guardian,
  workspace flock, manifest 디렉터리 fsync, 조회 projection 경쟁 직렬화.
- 최종 회귀 통과:
  - `go test ./...`
  - `CXZ_TEST_RACE=1 go test -race ./internal/... -count=1` (자식 바이너리도 race 빌드)
  - `go vet ./...`
  - `go run ./tools/genproto` (추적 중인 생성물과 diff 없음)
  - 실제 Claude live integration 재실행: 7개 경로, 40.17초, 모두 통과.
  - 최종 바이너리 Docker 재실행: 4개 검사 모두 통과.
    `testdata/recovery/cxz-container-final/summary.json`.
  - `git diff --check`
- 설계 결정: raw append-only 저널을 원본으로 유지하고 SQLite를 재구성 가능한
  조회 projection으로 사용한다. daemon/TUI 종료는 agent 종료와 분리한다.
- 설계 결정: 현재 개발 컨테이너 안에서 agent를 실행한다. 별도 devcontainer 자동
  provisioning과 사용자 로그인은 후속 작업이다. 컨테이너 자체가 사라진 경우
  프로세스 생존이 아니라 영속 상태에서 명시적으로 resume한다.
- 설계 결정: payday의 config/DB 및 gRPC 서버 구성을 사용한다. tenant/holder/ent
  CRUD 생성 대신 세션 전용 protobuf를 사용하며 인증 도입 시 별도 경계를 연결한다.

검증 결과와 실행 명령은 구현 단계마다 이 문서와 README에 갱신한다.

Git: 첫 기반 커밋 `2eaeda5`를 `lesomnus/cxz`의 `main`에 push했다.
HTTPS 인증은 없지만 기존 SSH 인증을 사용할 수 있어 push URL만 SSH로 설정했다.
두 번째 기능 커밋 `73b95c3`도 push 완료.
복구 보강 및 실제 검증 커밋 `df146cd`도 push 완료. 최종 검증 기록을 추가 커밋한다.

## 당시 남겨 둔 범위 (위 현재 단계에서 일부 구현됨)

사용자 인증/roster, 새 Claude 로그인 UI, 브라우저 UI, devcontainer 자동 생성,
파일·diff/IDE relay, 외부 암호화 백업, Codex adapter는 이번 TUI 수직 구현에
포함하지 않는다. 장기 아키텍처의 전체 MVP가 완료되었다는 의미는 아니다.
현재 journal은 세션 단위로 메모리에 재생하며 조회 시 파일을 스캔하므로
대용량 장기 운영에는 segment/index 및 보관 정책이 후속으로 필요하다.
