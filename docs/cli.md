# CLI 입력 계약

Linux/Windows 공통 원격 접속: `cxz connect [--token-file FILE] [--session SESSION] ENDPOINT`.
엔드포인트는 `ssh://user@host[:port]` 또는 `tcp://host:port`이며, Linux 호스트의 기존
설치에 `cxz expose --token-file FILE --listen tcp://127.0.0.1:7349`로 TCP 전달기를 열 수 있다.
`settings.jsonc`의 `connections`에 이름별 `target`과 `default`를 지정하면 기본
TUI에 모든 연결의 프로젝트·세션을 함께 표시한다 (`project1 via work`).
`cxz connect work`는 work에 초기 포커스하고, URL을 직접 주면 그 연결만 연다.
`local://`는 설치된 로컬 manager를 자동 탐색하고,
`unix://${STATE}/run/daemon.sock`는 foreground daemon의 소켓을 직접 사용한다.
`cxz expose`는 독립 릴레이이므로 manager 재시작이나 `install --expose`가 필요 없다.
Windows에서도 `cxz edit`로 로컬 설정을 편집한다. `VISUAL` → `EDITOR` → 메모장
순서로 선택하며, 메모장은 저장·닫기 후 터미널에서 Enter를 눌러 완료한다.
VS Code는 `$env:EDITOR = 'code --wait'`로 지정한다. Docker/file mapping 설정의
실제 적용은 Linux 호스트에서 수행한다.
`cxz edit share CLAUDE.md` 또는 `cxz edit share foo/bar/baz.txt`는
`<state>/share/` 아래 공용 파일을 편집하며 필요한 디렉터리를 만든다.
`files[].src`에서 `${CXZ_SHARE_DIR}`로 이 디렉터리를 지정할 수 있다.
Linux에서는 매핑된 공용 파일을 저장하면 다시 전송하며 다음 에이전트 시작 때 적용한다.
설정과 지원 범위는 [원격 접속](remote.md)을 참고한다.

`cxz self-update [--ref BRANCH_OR_TAG_OR_COMMIT] [--client-only]`는 Docker에서 소스를
빌드해 로컬 실행 파일과 설치된 Linux manager를 갱신한다. 기본 소스는 `lesomnus/cxz`의
`main`이며, Windows에서는 프론트엔드만 갱신한다. 자세한 조건과 복구 방법은
[소스 업데이트](operations.md#업데이트와-롤백)를 참고한다.

Windows에서는 `cxz self-install` (`cxz windows-install`도 동일)로 현재 실행 파일을
사용자 Programs 폴더의 `cxz\cxz.exe`에 복사하고 사용자 PATH에 등록한다.
기본 위치는 `%LOCALAPPDATA%\Programs\cxz\cxz.exe`이며 관리자 권한은 필요 없다.
설정 디렉터리는 유지하며, 기존 터미널은 완전히 종료하고 다시 열어야 새 PATH를 받는다.
설치한 실행 파일에서 `cxz integration add windows-terminal`을 실행하면 Windows Terminal에
cxz 프로필을 등록·갱신한다. `cxz integration ls`로 확인하고
`cxz integration remove windows-terminal`로 해제한다.
자세한 사용법은 [Windows 사용자 설치](remote.md#windows-user-installation-and-terminal-profile)를 참고한다.

목록 갱신은 변경 구독과 30초 안전 갱신을 사용한다. 매초 전체 목록/외부 컨테이너 탐색을 반복하지 않는다.
서버 측 동기화와 남아 있는 heartbeat의 범위는 [리소스 갱신과 부하](resource-refresh.md)를 참고한다.

## 컨텍스트 사용률

세션 하단 status bar에는 기존 quota 옆에 컨텍스트 사용률을 `⣄35%`처럼 표시한다.
11%마다 아래에서부터 점을 하나씩 채우며 88%부터 8점이 모두 찬다. 숫자는 99%까지만 표시한다.
미보고 상태는 `⠀—`이며 compact/새 run 이후에는 새 snapshot을 기다린다.
Codex는 CLI의 baseline 12,000 토큰 보정 기준을 따른 마지막 tokenUsage snapshot,
Claude는 마지막 assistant 메시지의 input+cache-read+cache-write와 해당 모델의 contextWindow를 사용한다.
누적 billing usage는 쓰지 않으며 `/context`를 자동 전송하지 않는다. `/context` 상세 출력은 변경하지 않는다.
Claude의 자동 갱신은 새 supervisor의 `context/message` 이벤트가 필요하므로 공유 runtime 업데이트 후 에이전트를 재시작해야 한다.
직접 실행한 `/context`의 `Context Usage`/`Model`/`Tokens` 보고서가 알려진 형식이면 명시된 비율도
status bar에 반영한다. 일반 대화 텍스트는 파싱하지 않으며 다음 입력/compact/새 run에서는 보고서 값을 무효화한다.

## 백틱 경로 힌트

대화 입력창에서 **백틱 뒤 `/` 또는 `~`**를 입력하면 해당 프로젝트 컨테이너의 경로 힌트가 뜬다.
일반 `/help` 등 대화 명령어와는 별개다. 예를 들어 다음처럼 입력한다(아직 백틱을 닫지 않은 상태):

```text
`/
`~/
이 파일 확인해줘: `/usr/lo
```

타이핑은 입력창에서 계속하고, ↑/↓는 힌트 항목을 선택한다. Tab은 선택한 경로 항목을 완성한다.
디렉터리는 `/`를 붙여 다음 디렉터리 목록을 연다. 파일을 선택해도 대화를 보내거나 파일을 읽지 않는다.
Enter는 선택한 경로를 완성하고 백틱을 닫는다(대화 전송 아님). 닫는 백틱을 직접 입력해도 탐색이 끝난다.
Esc도 닫으며 같은 입력에서는 즉시 다시 열지 않는다.
공백·한글 경로, 문장 중간/여러 줄 입력에서 주변 문장과 커서 위치를 유지한다.
디렉터리는 파란색, 일반 파일은 기본 글자색, 실행 파일은 초록색, 심볼릭 링크는 하늘색과 대상 경로로 표시한다.
검색에 매칭된 문자는 magenta로 강조한다. Ctrl+Backspace(kitty), Ctrl+W, Alt+Backspace는
열린 백틱 경로 안에서 `/` 경계까지만 지우고, 경계에서는 `/`와 앞 경로 이름을 다음 경계까지 함께 지운다.

50ms debounce 후 지속 연결된 `wisp`를 통해 remote user 권한으로 한 단계의 이름,
종류, 실행 권한, 링크 대상만 조회한다. `~`는 호스트나 agent private HOME이 아니라 컨테이너 remote user의 홈이다.
첫 항목부터 점진적으로 표시한다. 추가 결과는 현재 선택과 다음 항목까지의 순서를 보존하고 그 뒤에서 정렬한다.
검색어를 바꾸면 전체 후보를 다시 정렬한다. 첫 연결에만 컨테이너 소유권/실행 상태 검증과
`docker exec -i /cxz/tools/cxz wisp`가 필요하며, 이후에는 같은 stdio 연결로 Go 디렉터리 조회를 수행한다.
프로젝트 정보는 TUI가 가진 목록을 사용한다. 경로 조회마다 전체 프로젝트 RPC를 호출하지 않는다.
조회 중 입력/프로젝트가 바뀌거나 탐색을 닫으면 요청을 취소하고 늦게 온 결과를 무시한다.
동일 디렉터리 안의 fuzzy 필터링은 cxz 내장 검색을 사용하므로 fzf 설치·바이너리 주입은 하지 않는다.
디렉터리당 최대 2,048개 항목을 조회하며 한도 초과를 표시한다. 제어 문자·유효하지 않은 UTF-8 이름은
제외하고, 백틱이 들어간 이름은 자동 완성하지 않는다. 터미널 패널처럼 로컬 Docker 접근이 필요하다.

기존 환경은 로컬 CLI와 `/cxz/tools/cxz` 공유 도구 바이너리에 모두 `wisp`가 포함되어야 한다.
새 CLI로 `cxz install --recreate`하여 공유 도구를 갱신한 뒤 TUI를 다시 열면 된다.
wisp 도입 자체 때문에 프로젝트 writable layer를 지우는 `project recreate`는 필요하지 않다.
외부 포트를 열거나 에이전트 인증을 요구하지 않는다. TUI 종료 시 helper 연결을 종료한다.
자세한 수명·프로토콜·검증 범위는 [wisp 설계](wisp.md)를 참고한다.

## 세션 하단 컨테이너 터미널

세션에서 `Ctrl+백틱` 또는 `/terminal`을 사용하면 하단에 최대 24줄 PTY와 위아래 구분선이 열린다.
작은 창에서는 대화·입력·승인 영역을 남기고 높이를 줄인다. 터미널 포커스에서 같은 키를 누르면
접고 대화 입력창으로 돌아온다. 이미 보이지만 포커스가 대화에 있으면 터미널로 포커스만 이동한다.
Kitty의 `CSI 96;5u`를 지원하며, 이 키를 전송하지 않는 터미널에서는 `/terminal`을 사용한다.
내부 키 변환용 F20도 예약한다. 붙여넣은 단축키 문자열은 동작시키지 않는다.

상단 구분선 오른쪽 `[Collapse ▾]`는 패널을 접는다. 별도 정보 줄은 표시하지 않는다.
입력창/터미널 본문 클릭으로 포커스를 전환한다. 위아래 구분선은 여백 없이 본문 폭을 채운다.
Collapse 버튼은 내부 프로그램의 마우스 모드와 별개로 cxz가 처리한다. 터미널 포커스 중에는
Ctrl+C/Tab/Ctrl+S 등을 셸로 전달하므로 cxz 대화 단축키로 처리되지 않는다.

하단 구분선은 `◆︎` 핸들이 있는 스크롤바다. 휠, Alt+PgUp/PgDown, 스크롤바 클릭/드래그로
최대 2,000줄의 보관 이력을 탐색한다. 탐색을 시작하면 snapshot을 유지하여 새 출력이나
오래된 이력 제거로 화면이 움직이지 않는다. 오른쪽 끝 클릭, 탐색 중 Ctrl+End 또는 타이핑은
최신 출력으로 복귀한다. alternate-screen 앱에서는 본문 휠을 앱에 전달한다.

커서는 대화 입력창과 같은 연두색 깜빡이는 가상 커서이며, 커서만 이동해도 화면이 갱신된다.
IME 위치를 위한 실제 커서 좌표는 계속 동기화한다. 새 셸의 locale이 UTF-8이 아니거나 유효하지
않으면 설치된 C.UTF-8/C.utf8/en_US.UTF-8 중 사용 가능한 것을 선택한다. 사용자 rc 파일은
수정하지 않으므로 rc에서 locale을 다시 바꾸는 경우 해당 설정을 확인해야 한다.

셸은 세션별로 하나씩 만들며 접기·세션 전환 중에도 유지된다. 종료 코드 0이면 패널을 자동으로
접고 대화 입력으로 복귀한다. 0이 아닌 종료는 화면을 유지한다. 다시 열면 새 셸을 만든다.
프로젝트의 remote user와 remote workspace로 실행하고 provider의 private
authentication HOME은 사용하지 않는다. 이 터미널 명령은 에이전트 approval을 거치지 않는
직접 사용자 명령이다. 입력·출력은 대화 이벤트 저널에 기록하지 않는다.

현재 transport는 로컬 Docker CLI의 `exec -it`이며, cxz manager와 같은 Docker engine에 접근해야
한다. 실행 전에 실제 container ID·project/owner label·running 상태를 확인한다. 원격 manager만
접근 가능한 환경에서 별도의 terminal RPC relay는 제공하지 않는다. cxz detach 시 연결을 닫으므로
앱 재접속을 넘는 셸 복구는 지원하지 않는다. 의도적으로 분리한 백그라운드 프로세스까지 종료를
보장하지 않으므로 필요하면 셸에서 먼저 종료한다. 접기는 종료가 아니다.

터미널 출력은 [Charm VT emulator](https://github.com/charmbracelet/x/tree/main/vt)로 해석하여
커서 이동·alternate screen·컬러·마우스·bracketed paste를 패널 안에서 처리한다. 크기 변경을 PTY에
전달하며 출력 갱신은 최대 20fps로 합친다. 모든 터미널 확장/그래픽 프로토콜 지원을 의미하지 않는다.

## CLI 자동 업데이트

설치된 manager는 **기본으로 자동 적용**한다. 최초 실행 시와 마지막 확인으로부터 24시간마다
Claude/Codex/gh의 공식 최신 stable 버전을 확인한다. prerelease는 제외하며 기존보다 높은
버전만 적용한다. checksum 검증한 immutable tools cache에 다운로드하고 컨테이너에서
`--version` 실행을 확인한 후, 전체 프로젝트에서 한 번에 한 세션씩 업데이트·재시작한다.
대기 큐는 30초마다 검사하며 새 세션도 확인된 release channel을 사용한다.

적용 조건은 연속 5분 idle이다. 실행 중인 turn/tool, background task/자식 프로세스,
pending approval/question, model/effort 변경, TUI 입력 활동이나 미전송 초안이 있으면 보류한다.
TUI는 10초 heartbeat와 휴지 후 첫 입력을 보고한다. 연결이 끊기면 30초 lease 만료 후
새 idle 구간을 기다린다. 입력 보고가 없는 구버전 클라이언트가 붙어 있거나 알 수 없는
background 이벤트/프로세스 상태가 있으면 적용하지 않는다. 상주 MCP/LSP 등의 자식 프로세스도
보수적으로 보류 사유가 될 수 있다. 서버에서 받아들인 새 입력과 업데이트 중단은 같은 잠금으로
직렬화한다. 재시작과 겹쳐 전송에 실패한 TUI 초안은 복원하며 메시지를 자동 재전송하지 않는다.

업데이트는 기존 session/account/auth binding과 provider 대화 이력을 유지하는 resume이다.
새 프로세스 초기화 실패 시 이전 실행 파일로 복구한다. runtime 중단에 대비한 transaction을
세션 디렉터리에 저장하며 재기동/주기 검사에서 복구한다. 실패한 버전과 실패한 복구 재시도는
24시간 간격으로 제한한다. 세션에 저장한 `/permission full|ask` 정책은 재시작 후에도 유지된다.
gh는 cxz-managed wrapper의 실행 파일 링크만 원자적으로 교체하고 에이전트를 재시작하지 않는다.
이미지에 설치된 사용자 gh 및 과거 고정 경로 wrapper는 자동 교체 대상이 아니다.

세션 대화에는 queued/applying/completed/rollback 알림이 표시된다. manager 컨테이너의
`/var/lib/cxz/updates.json`에 마지막 확인 시각, 버전, 확인 오류와 세션별 대기/실패 사유를 저장한다.
확인 실패 시 이미 실행 중인 세션은 계속 동작한다. 호스트 CLI, cxz manager/runtime 이미지,
프로젝트 이미지 자체와 인증 토큰은 이 worker가 업데이트하지 않는다.

최초 도입은 새 CLI로 `cxz install --recreate` 후 필요한 프로젝트를
`cxz project recreate WORKSPACE`하여 manager/runtime/supervisor를 함께 갱신한다.
구버전 runtime을 안전 조건 확인 없이 강제 재시작하지 않는다. 이후 provider CLI 자동 적용에는
프로젝트 재생성이 필요 없다. 비활성화하려면:

```sh
CXZ_AUTO_UPDATE=false cxz install --recreate
```

다시 활성화하려면 `CXZ_AUTO_UPDATE=true cxz install --recreate`한다.

## 컨테이너 GitHub CLI와 호스트 인증

프로젝트 준비 시 GitHub CLI 2.100.0 공식 Linux amd64/arm64 아카이브를 checksum 검증 후
공유 tools volume에 캐시한다. 프로젝트에 `gh`가 없으면 `/usr/local/bin/gh` wrapper를 설치하며
이미지에 있는 `gh`는 덮어쓰지 않는다. 인증은 바이너리 cache나 이미지에 포함하지 않는다.

`cxz install`, `cxz up`, 프로젝트 준비/새 세션 작업에서 호스트 인증 snapshot을 manager에
동기화한다. `GH_CONFIG_DIR`, `XDG_CONFIG_HOME`, 기본 `~/.config/gh` 순서로 설정을 찾고
각 호스트의 활성 토큰을 `gh auth token --hostname HOST`로 읽는다. 키체인 인증도 이 경로를
사용하며, 호스트 gh가 없으면 hosts.yml의 평문 토큰을 사용할 수 있다. GH_TOKEN 계열 환경변수는
gh와 같은 우선순위로 적용한다. 토큰 값과 인증 명령 stderr는 출력하지 않는다.

snapshot은 Docker stdin으로 manager의 `/var/lib/cxz/host-gh/hosts.yml`에 전달한다.
프로젝트에는 `/cxz/state/data/gh/hosts.yml`로 복사하고 `GH_CONFIG_DIR`로 연결한다.
디렉터리 0700, 파일 0600이며 프로젝트 사본은 remote user 소유다. 호스트 경로 bind mount나
Docker 환경변수/명령 인수에 토큰을 넣지 않는다. Claude/Codex의 분리된 HOME에서도 같은
프로젝트 GitHub 인증을 사용하며 provider account별 GitHub 계정을 선택하는 기능은 아니다.
프로젝트 안에서 실행되는 도구는 이 인증을 사용할 수 있으므로 신뢰하는 workspace에서 사용한다.

기존 환경의 최초 적용: CLI와 manager를 업데이트(`cxz install --recreate`)하고 필요한 프로젝트를
`cxz project recreate WORKSPACE`로 재생성한다. 기존 사용자 gh, GH_CONFIG_DIR 환경변수와 관련된
차이까지 적용하려면 재생성이 필요하다. 이후 호스트 로그인 변경/로그아웃 후 `cxz up WORKSPACE`는
해당 실행 중 프로젝트의 credential snapshot을 다시 복사하며, 로그아웃한 인증은 빈 snapshot으로
제거한다. 다른 프로젝트는 각각 up/재준비할 때 갱신된다. 상시 토큰 갱신 broker는 아니다.
`gh auth setup-git`, SSH 키 전달, 사용자 gitconfig 전체 복사는 자동 수행하지 않는다.

참고: [gh 환경변수](https://cli.github.com/manual/gh_help_environment),
[gh auth token](https://cli.github.com/manual/gh_auth_token).

## 뷰 폭과 프로젝트 패널

뷰의 본문 최대 폭은 133칸이며 초과 영역은 오른쪽에 비워 둔다. 터미널 가로가
70칸 이상(높이 14줄 이상)이면 왼쪽에 프로젝트 패널과 2칸 간격을 추가한다.
패널 폭은 터미널 너비의 1/4로 잡되 최소 28칸, 최대 36칸으로 제한한다.
80칸 터미널에서는 패널 28칸과 대화 50칸이 함께 표시되며, 대화에는 최소 40칸을 확보한다.
프로젝트·세션·accounts·로그인 workflow·모달 모두 제한된 본문 안에 표시된다.

패널에는 cxz에 등록된 모든 프로젝트와 그 프로젝트의 세션이 표시된다. 목록은 주기적으로
갱신되며 방향키와 PgUp/PgDn, Home/End로 탐색할 수 있다. 프로젝트 행에서 Enter는 해당
프로젝트의 첫 세션 행으로 이동하고, 세션 행에서 Enter는 해당 대화를 연다. 프로젝트 간 이동에서도
세션별 초안을 보존한다. 다른 프로젝트의 세션 생성/로그인은 그 프로젝트를 대상으로 한다.
행 전체를 클릭하면 Enter와 같은 동작을 수행하며, 마우스 휠로 목록을 탐색할 수 있다.
선택 행은 ANSI 238, hover 행은 ANSI 237 배경을 빈 공간까지 채우고 기존 글자색을 유지한다.
Hover만으로 키보드 포커스나 현재 세션이 바뀌지는 않는다.
대화 본문이나 입력창을 클릭하면 프로젝트 패널에서 대화로 포커스를 돌려받는다.
목록에서 n/Ctrl+N과 a는 선택 행의 프로젝트에서 새 세션 생성과 계정 관리를 연다.
r은 선택한 세션의 이름을 패널에서 편집한다. s와 d/Delete는 선택한 세션만 중지하거나 확인 후 삭제한다. 프로젝트 행에서는
세션을 먼저 선택하도록 안내한다. 좁은 전체 화면과 넓은 사이드 패널 모두
외곽 테두리 없이 어두운 회색(#303030) 배경과 상하좌우 한 칸 패딩으로 구분한다.
사이드 패널에 포커스가 있을 때만 하단 선을 그린다.
시작 명령의 `--trust-config`는 원래 프로젝트에만 적용하며 다른 프로젝트로 전파하지 않는다.

대화 중 Tab/Shift+Tab은 승인 영역과 입력창 사이를 순환한다. 입력창 아래 왼쪽에는 활성화된 FULL만, 오른쪽에는 quota/context를 표시한다.
Ctrl+Q는 화면 폭과 관계없이 같은 프로젝트·세션 탐색 목록에 포커스를 준다.
70칸 이상에서는 대화를 유지한 채 왼쪽 패널로 포커스만 옮기고,
70칸 미만에서는 별도 전체 뷰로 표시한다. 탐색 중 창을 넓히거나 좁혀도
선택 행과 포커스를 유지하며 표현만 전환한다. Tab은 포커스를 이동하지 않고
Esc/Ctrl+Q는 원래 뷰로 돌아간다. 대화 초안과 보고 있던 세션은 탐색 진입만으로 바뀌지 않는다.

## 긴 붙여넣기

일반 입력창과 선택 질문의 Other에서 800자 초과 또는 4줄 이상 붙여넣기는 칩으로
접어 표시한다. 기본값은 **원문 전송**이며 파일로 자동 변환하지 않는다. 터미널이
bracketed paste를 제공해야 직접 입력과 구분할 수 있다. 로그인 코드/secret 질문은 제외한다.

칩에 좌우 방향키로 진입하면 전체를 선택하고, 다음 방향키로 칩 밖으로 이동한다.
선택된 칩에서 Enter는 메뉴를 열며 Backspace/Delete는 칩 전체를 제거한다.
선택된 칩에서는 `t` 원문 모드, `f` 파일 첨부 전환, `d` 삭제가 바로 동작한다.
선택을 해제하면 다시 일반 문자로 입력된다. 붙여넣은 `t/f/d`는 단축키로 실행하지 않는다.
모드 전환은 대화를 즉시 전송하지 않는다.

`/paste`로 미리보기를 연다. `↑/↓`는 paste 선택, `PgUp/PgDn`은 미리보기 스크롤,
`t`는 원문 전송, `f`는 파일 첨부 전환, `d`는 초안에서 제거, `Esc`는 닫기다.
/paste 미리보기는 현재 입력에 남아 있는 칩만 표시한다. 삭제/전송한 칩은 캐시에서
다시 표시하지 않는다. `Ctrl+P`는 공유 Docker 상태와 관리 액션이 있는 설정 화면을 연다.

파일 전환은 현재 세션의 runtime 영구 저장소에 업로드한 뒤 적용된다. `[File …]` 칩은
원문 대신 에이전트가 읽을 수 있는 경로와 읽기 안내를 전송한다. 원문으로 되돌릴 수 있으며,
초안에서 칩을 지워도 이미 저장한 파일은 삭제하지 않는다. 파일 읽기에는 기존 provider 권한이
적용된다. 업로드 기능은 CLI·manager·프로젝트 runtime 서버 업데이트가 모두 필요하다.

붙여넣기 하나/첨부 파일은 최대 1 MiB, 메모리 paste cache는 32 MiB다. 초안과 paste cache는
앱을 종료하면 사라지지만, 명시적으로 업로드한 파일은 세션 데이터에 남는다.


## 기본 TUI와 도구 미리보기

인자 없이 `cxz`를 실행하면 `cxz tui`와 같은 TUI를 연다. 도움말은 `cxz --help`다.
Bash 명령은 셸 구문을 강조한다. Write/Edit 또는 Codex 파일 변경 행을 클릭하면
당시 도구 입력을 미리 본다. 좁은 화면에서는 입력창 위에 최대 6줄, 충분히 넓은
화면에서는 오른쪽 패널에 표시한다. 패널 위에서 마우스 휠로 스크롤하고 `[×]`로 닫는다.
현재 파일을 다시 읽는 것이 아니므로 이후 변경과 섞이지 않는다.

Codex가 `item/commandExecution/outputDelta`를 보내면 실행 중인 명령 아래에 최신
출력을 최대 6줄 표시하고, 완료 시 접는다. 중간 출력을 제공하지 않는 도구/공급자는
완료 이벤트를 기다린다. Claude의 진행 시간 알림을 실제 명령 출력으로 취급하지 않는다.

선택한 세션만 대화 이벤트를 구독한다. 다른 세션도 에이전트 실행과 저널 기록은
계속된다. 돌아올 때 누적 이벤트는 페이지 단위로 반영한 뒤 실시간 구독을 이어간다.
질문에서 이미 선택한 항목을 Enter로 다시 선택하면 Next/Submit에 포커스가 이동하며,
실제 제출에는 한 번 더 확인이 필요하다. 복수 선택의 Space는 선택/해제를 유지한다.

## Status bar quota

Codex의 status bar는 현재 모델과 일치하는 모델별 한도를 우선 표시하고, 일치하는
한도가 없으면 일반 Codex 한도를 표시한다. Claude는 `5h · wk`를 우선 표시한다.
선택되지 않았거나 화면 폭 때문에 생략한 한도는 `+N limits`로 나타낸다. 전체 내역은
`/usage`에서 확인한다. 접힌 한도가 현재 모델에 적용되지 않는다고 단정하는 표시는 아니다.

모델 이름을 quota에서 반복하지 않으며, 화면이 좁으면 bar부터 줄이거나 생략한다.
세션 식별 정보 공간과 오른쪽 한 칸 여백은 우선 확보한다.


## Codex 비동기 질문

Codex의 `agentMessage`에 `delivery: async`와 `questions`가 있으면 선택 다이얼로그가
열린다. 턴이 끝나 idle이 되어도 해당 run에서 답변할 수 있다. Esc는 창만 닫고,
`/answer`로 다시 열 수 있다. 단일 선택 또는 Other를 제출하면 질문과 선택 정보를
Codex에 tool output으로 전달한다. Claude의 다중 선택 질문도 기존대로 지원한다.

blocking 질문은 기존 승인 응답 경로를 사용하지만, async 질문은 작업을 멈추지 않는다.
중단/에이전트 재시작으로 run이 끝난 질문은 만료된다. CLI뿐 아니라 runtime/supervisor를
업데이트해야 새 이벤트가 pending 질문으로 전달된다.


## TUI 안에서 세션 생성과 로그인

프로젝트 목록에서 `n`으로 account를 선택하면 전체화면을 나가지 않고 세션 준비 상태를
표시한다. 로그인 URL도 같은 화면에 표시하며 Claude의 반환 코드는 입력 후 Enter로
제출한다. 입력 내용은 숨기고 글자 수만 표시한다. Ctrl+X는 입력 초기화, PgUp/PgDn은
출력 스크롤, Esc는 로컬 작업 취소다. 이미 준비된 서버 리소스를 삭제하지는 않는다.

프로젝트의 `a`로 accounts 뷰를 열고 `l`로 로그인할 수도 있다. Codex는 계정 단위로
로그인한다. Claude는 세션별 인증이므로 대상 세션을 선택하거나 `New session + independent
login`을 선택한다. 기존 세션의 재로그인은 프로젝트 목록에서 해당 세션을 먼저 중단해야 한다.


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
| `up [WORKSPACE]` | 기본 `.`. 프로젝트 컨테이너를 준비한 뒤 `cxz`로 TUI를 열라는 안내를 출력하고 종료. 이미 실행 중이면 재준비하지 않음. 세션 생성/Account/로그인 요구 없음. `--format json\|table` 또는 `--no-attach` 명시 시 안내 대신 프로젝트 정보 출력 |
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
`cxz edit docker-compose`로 프로젝트 공통 Compose override를 편집한다.
기본 파일은 `settings.jsonc` 옆의 `docker-compose.yaml`이며, 없으면 주석 예시와
함께 생성한다. 다른 경로가 필요할 때만 `cxz edit`에서 `devcontainer.compose`를
문자열로 지정한다. 여러 파일은 Compose의 `include`로 구성할 수 있다.
호스트 CLI가 편집 저장 및 준비 명령마다 포함한 파일까지 읽어 manager에 저장하며,
새 컨테이너 또는 명시적으로 재생성한 컨테이너에 적용한다.
설정 예시와 변수는 [프로젝트 Compose override](devcontainer-overrides.md)를 참고한다.
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

최상위 up은 컨테이너 준비 후 안내만 출력하고 종료하며 TUI를 열지 않는다.
`--format json`은 `--no-attach` 없이도 프로젝트 JSON을 출력한다. `--no-attach`도
기존 스크립트 호환을 위해 유지하며 프로젝트 정보를 출력한다.
프로젝트·세션 목록은 `cxz` 또는 `cxz tui`로 연다.
별도 프로젝트 전용 화면은 없으며 같은 목록으로 시작·탐색·세션 관리를 수행한다.
↑/↓·Enter로 세션을 열고, n/Ctrl-N은 계정 선택·첫 로그인·새 세션 생성으로 이어진다.
프로젝트당 실행 세션 하나 제약은 유지하므로 기존 세션을 s로 중지한 뒤 새로 만든다.
d/Delete는 확인(y, Esc/n 취소) 후 선택한 세션을 중지·삭제한다. 확인 중 목록이 바뀌어도
처음 지정한 ID만 삭제한다. Ctrl-Q는 프로젝트·세션 탐색 화면을 열며 agent를
중지하거나 프롬프트를 전송하지 않는다. Ctrl-C는 TUI만 닫는다.
it은 세션 화면을 선택해 열 뿐 중지된 agent를 자동 재개하지 않는다(Ctrl-R로 재개).

프로젝트 목록은 프로젝트와 그 아래 세션 행을 표시하고 선택한 행을 강조한다.
세션 화면은 대화 본문, 승인/스크롤 상태, 여러 줄 입력창, 단축키 안내를 분리한다.
Enter/Alt-Enter/Ctrl-J는 줄바꿈, Ctrl-Enter/Ctrl-S는 전송, Ctrl-X는 입력 지우기다.
Windows 빌드는 콘솔 이벤트의 Ctrl 상태로 Enter를 구분하므로 별도 키보드 프로토콜 설정 없이
Ctrl-Enter로 전송한다. 왼쪽/오른쪽 Ctrl과 숫자 키패드 Enter도 처리하며 입력 모드는 종료 시 복원한다.
Unix에서는 CSI-u 또는 xterm modified-Enter를 보내는 터미널에서 구분된다. 기존 CR만
보내는 터미널에서는 Enter와 구분할 수 없으므로 Ctrl-S를 사용한다. Kitty 전체 키보드
프로토콜을 강제로 켜지는 않는다. 여러 줄 붙여넣기는 전송하지 않고 편집기에 남는다.
Esc는 대화 초안을 지우지 않는다. 선택/생성/승인 화면의 Enter 동작은 그대로다.
프로젝트로 돌아가거나 다른 세션을 선택해도 각 세션 초안은 현재 TUI 프로세스 안에서
유지되며, 종료 후에는 보존하지 않는다. PgUp/PgDn은 입력 커서를 움직이지 않고 본문만
스크롤한다. 최소 터미널 크기는 40 × 14이며 입력창/테두리/현재 입력 줄도 터미널
배경색을 유지한다. 입력창은 첫 줄 >, 다음 줄부터 어두운 1…9, 0, 1… 한 자리 번호를 표시한다.

브랜드 팔레트는 #24d17c / #031e2c / #aeff98 / #07898f이며 도구·진단에는
파스텔 보조색을 쓴다. Claude 이름은 테라코타색, Codex 이름은 검정 배경/흰 글자로
표시하며 대답 본문은 회색, 굵은 글씨는 밝은 회색으로 표시한다. 입력창 테두리는 좌우 마진 없이 터미널 너비를 채운다. 사용자 메시지는
YOU 대신 로컬 시간대의 MM-DD HH:mm 시각(흐린 색)과 > 접두사로 표시한다.
시간과 본문을 ANSI 236 배경으로 함께 감싸고 상하 한 줄씩 패딩을 둔다. 배경은 대화 영역의 좌우 끝까지 채운다.
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
미응답 요청이 있는 동안에는 중복 조회를 억제한다.
Claude는 동일한 cxz Account 프로필의 세션들 사이에서 분당 한 번의 조회 권한과
마지막 정상 응답을 공유한다. manager의 quota 소켓을 통해 프로젝트 간에도 공유하고,
다른 세션은 약 5초 간격으로 공유 캐시를 확인한다. 공급자 요청을 세션마다 보내지 않는다.
조회 세션이 종료되면 권한은 1분 후 만료되어 다른 세션이 이어받는다.
빈 응답이나 수치 없는 상태 알림은 기존 숫자를 지우지 않으며 오래된 값은 `~`로 표시한다.
프로필이 다른 계정은 합치지 않는다. 이 기능에는 manager와 supervisor 업데이트가
필요하며 이전 manager에서는 각 세션이 분당 조회하는 방식으로 동작한다. `/usage`에 마지막 quota 요청 시각을
표시하므로 매분 폴링 여부를 확인할 수 있다. 공급자 메서드 미지원은 unsupported,
계정에서 한도 미제공은 unavailable, 조회 실패는 error로 표시한다. /usage에 원인별
안내와 스냅샷 관측/reset 시각을 덧붙인다. 상태바 오른쪽은 한 칸 비운다. 2분 이상 오래된 값은
~로 표시하며 오류/타임아웃 때도 이전 값에 ~를 붙인다. 계정 이름 앞 장식 심볼은 생략한다.
reset 시각이 지나도 100%로 추정하지 않고 refresh를 표시한다.
좁은 화면에서는 제목을 먼저 줄이고 추가 한도는 +로 표시한다.

### 승인·스크롤·대화 렌더링

접속 시 최근 128개 이벤트를 한 번에 읽어 마지막 화면부터 표시한다. 최초 응답을 기다리는
동안 대화 영역에 움직이는 skeleton을 표시한다. 첫 화면 뒤에는 이전 페이지 하나를 미리 읽고,
스크롤 위치가 로드된 기록의 위쪽 끝에서 여섯 화면 이내로 들어오면 이전 기록을 128개씩
추가로 읽는다. 새 기록을 붙여도 보고 있던 줄의 위치를 유지한다. 화면에 표시할 내용이
부족하면 다음 페이지를 이어서 읽는다. PgUp/Ctrl+Home/마우스 휠을 지원한다.
Markdown과 작업 행은 백그라운드에서 미리 렌더링하고, 기존 Bash 구문 강조와 작업 해석 결과도
재사용한다. 녹화에는 `history_rpc`, `history_prepare`, `history_first_render` 시간을 기록한다.
아직 전체 기록을 읽지 않았다면 줄 번호는 `loaded L`로 표시하며 전체 저널의 절대 줄
번호가 아니다. 읽은 페이지와 렌더링 캐시는 현재 TUI에서 유지한다. 이는 TUI의 지연
로딩이며 서버의 원본 저널 재생/SQLite projection 자체를 바꾼 것은 아니다.

Markdown fenced/indented 코드 블록은 ANSI 238 배경과 내부 한 칸 여백으로 표시한다.
표는 외곽선 없이 헤더 아래에 ANSI 22의 굵은 선, 데이터 행 사이에 ANSI 240의 얇은 회색 선을
그리며 열 사이에는 한 칸을 비운다. 도움말은 감지된 색 프로필을 표시한다. True Color에서는 홀수/짝수 키캡을
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

### 작업 내용 키보드 탐색

`/view`는 대화에 표시된 작업 줄에 선택자를 표시한다. ↑/↓로 이동하고 Enter로
클릭과 동일한 내용 패널을 연다. Home/End는 현재 로드한 첫/마지막 작업을 선택한다.
첫 작업에서 ↑를 누르면 이전 기록을 요청하고, 로딩 후 다시 ↑로 이전 작업을 선택한다.
새 이벤트가 도착하거나 화면 크기가 바뀌어도 선택은 같은 저널 이벤트에 남는다.

Write/Edit는 제출한 내용과 diff, Read는 기록된 읽기 결과를 보여준다. 그 밖의 작업도
결과 텍스트 또는 원본 JSON을 조회한다. 현재 디스크 파일을 다시 읽지 않는다.
좁은 화면에서는 입력창 위에 내용 16줄을 표시한다. 내용이 짧아도 16줄 공간을 유지하며
작은 터미널에서는 입력창을 보존하도록 높이를 줄인다. 넓은 화면의 오른쪽 패널은
내용 길이와 관계없이 화면 전체 높이를 차지하며 스크롤·PgUp/PgDn도 그 높이를 따른다.
작업 패널은 상단 공백 없이 제목으로 시작하고 어두운 회색(#303030) 배경과 좌우 한 칸 패딩을 사용한다.
외곽 테두리 없이 포커스가 있을 때만 하단 선을 표시한다. 제목 줄의 ⧉ 또는 Ctrl+C로 원문 전체를 복사한다.
× 또는 x/Esc로 닫으며 ⧉/×에는 마우스 hover 효과가 있다.

마우스나 Enter로 열면 패널에 키보드 포커스가 이동한다. ↑/↓, PgUp/PgDn,
Home/End 또는 휠로 스크롤하고 `x`, Esc, ×로 닫는다. `/view`에서 연 경우
닫거나 Tab을 누르면 작업 선택으로 돌아간다. 선택 모드에서 Esc 또는 Tab을
누르면 입력창으로 돌아간다. 마우스로 연 패널에서 Tab을 누르면 입력창으로
돌아가며, 패널을 다시 클릭하면 포커스를 받는다. 붙여넣은 문자는 단축키로 처리하지 않는다.

대화 본문을 드래그하여 선택한 뒤 Ctrl+C로 복사할 수 있다. 복사는 터미널의 OSC 52
클립보드를 사용하므로 원격 SSH에서도 사용자의 터미널로 전달된다. 터미널이 OSC 52를
허용해야 하며 성공 여부는 클라이언트에서 확인할 수 없어 요청 상태를 안내한다.
Ctrl+D는 TUI를 detach하고 에이전트는 계속 실행한다. 컨테이너 셸에 포커스가 있으면
Ctrl+C/Ctrl+D는 기존 셸 키로 전달된다.

F9 녹화는 특정 세션에 고정되지 않고 현재 TUI 전체의 입력·렌더링·성능 진단을 수집한다.
세션 전환도 같은 기록에 들어가지만 대화 본문은 수집하지 않는다. 녹화 중에는 입력창
바로 위 오른쪽에 빨간색 `⬤ REC`를 표시하며 점만 1초 간격으로 켜고 끈다.

### TUI 진단 로그 오버레이

`/logs`는 현재 세션의 TUI 알림 원문, 저널의 진단·상태·업데이트 기록,
supervisor 로그와 agent stderr를 보여준다. 긴 오류 문구는 화면 너비에 맞춰
줄바꿈하며 ↑/↓, PgUp/PgDn, 마우스 휠로 스크롤한다. Home/End로 처음/끝으로
이동하고 `r`로 새 스냅샷을 읽으며 Esc로 닫는다. 대화 스크롤 위치는 유지한다.

`/logs project`는 현재 프로젝트의 다른 세션, runtime, provisioning,
컨테이너 stdout/stderr 및 이 TUI가 실행한 Wisp의 stderr·시작/종료 진단도 포함한다.
출처별로 묶은 최근 로그이며 전역 manager 로그는 `cxz manager logs`로 확인한다.
서버가 구버전이거나 일부 파일/컨테이너에 접근할 수 없으면 해당 출처에 실패 이유를
표시하고 나머지 로그는 유지한다. 서버 로그 RPC에는 최신 manager/runtime이 필요하다.

TUI 알림은 현재 TUI 실행 중 최근 200개를 보관하며 재실행 후 복구하지 않는다.
Wisp 진단도 이 TUI 실행 중 프로젝트별 최대 64 KiB만 보관한다. 다른 TUI 연결이나
이전 실행의 Wisp 로그는 수집되지 않는다. Wisp stdio 요청/응답 본문은 기록하지 않는다.
서버 파일은 출처별 최대 200줄/64 KiB, 저널은 마지막 1 MiB의 진단 이벤트 최대
200개를 읽는다. 전체 보고서는 약 1 MiB로 제한하며 생략 여부를 표시한다.
프로젝트를 삭제·재시작하거나 세션에 명령을 보내지 않는 읽기 전용 기능이다.

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

대화 아래에 빈 줄을 둔다. 승인 영역은 대화와 입력창 위 안내 줄 사이에 공간을 차지하며
상하 구분선으로 구별한다. 오버레이가 아니므로 대화를 가리지 않는다.
Tab은 승인(있을 때)과 입력창 사이를 순환하며 Shift-Tab은 역순이다. 세션 선택은 Ctrl+Q 프로젝트 패널에서 한다.
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

/permission full은 해당 세션의 자동 승인 정책을 supervisor 저널에 저장한다.
이미 대기 중인 요청과 이후의 알려진 도구·명령·파일·권한 요청은 supervisor가 처리한다.
다른 세션을 보거나 TUI를 종료해도 자동 승인은 계속된다. manager 재시작과
supervisor 재시작/resume, 세션 볼륨을 유지한 컨테이너 재생성 후에도 정책을 복원한다.
새 세션과 정책 기록이 없는 기존 세션은 full로 시작한다. 명시적으로 저장한 ask는 유지하며
계정이 같아도 다른 세션의 정책을 상속하지 않는다.
질문·미지원 요청은 수동으로 남긴다. /permission ask는 수동 승인 정책을 저장하며
이미 전송한 결정은 되돌리지 않는다. 실행은 기존 run/request 검사와 저널 기록을
공유하고 모호한 전송 실패는 자동 재시도하지 않는다. 공급자 sandbox 설정은 바꾸지 않는다.

TUI는 Permission RPC와 상태 표시만 담당한다. 실제 프로세스 수명과 승인 처리는
프로젝트 runtime/supervisor에 속한다. Wisp는 경로 탐색 등 연결형 workspace helper다.
이 변경은 manager/runtime 업데이트와 기존 supervisor 재시작이 필요하다.
이전 TUI의 로컬 설정은 옮기지 않는다. 저장된 정책이 없으면 기본값 full을 사용한다.

`unknown method Permission`은 요청을 받은 manager 또는 프로젝트 runtime이 구버전이라
새 RPC를 제공하지 않는다는 뜻이다. 호스트에서 최신 cxz 바이너리로
`cxz install --recreate` 후 `cxz project recreate WORKSPACE`를 실행한다.
install만으로는 살아 있는 프로젝트 runtime/supervisor가 교체되지 않는다.
project recreate는 워크스페이스와 named volume을 유지하지만 컨테이너 writable layer를
삭제하고 편집기 연결을 끊는다. 컨테이너 내부에만 있는 파일은 먼저 보관한다.
중단 상태로 남은 세션은 TUI에서 Ctrl+R로 재개한다.

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
Windows 콘솔에서는 modifier 정보로, Unix에서는 구분된 키 시퀀스로 처리한다.
Unix TUI는 alternate screen에 있는 동안 Kitty disambiguation 모드를 요청한다.

응답 프로토콜의 text에는 확정적 Markdown 형식 표시가 없다. 문법이 발견되면
CommonMark/GFM으로 렌더링한다. 제목/목록/표/강조/코드를 지원하고 백틱 코드 구간은
ANSI 238 배경이다. 코드 블록은 상하 여백 한 줄과 오른쪽 위 ⧉ 복사 버튼을 제공한다.
화면 폭에 따른 줄바꿈이나 구문강조를 포함하지 않고 코드 원문을 복사한다. HTML·외부 이미지 fetch·OSC 링크 실행은 하지 않으며 원본 journal은 유지한다.
물리 터미널 커서를 위젯의 실제 커서 칸으로 맞춰 IME 후보/조합 위치를 보정한다.
줄바꿈/한글 셀 너비/스크롤/NO_COLOR를 고려하고 OAuth 화면에는 보정을 적용하지 않는다.
사용자 OS IME의 미완성 글자 조합은 사용자 터미널에서 재확인이 필요하다.
입력창 아래 왼쪽에는 활성화된 FULL만, 오른쪽에는 quota/context를 표시한다. 세션 이름과 에이전트는 왼쪽 패널에서 확인한다.
좌측 사이드바 하단에 오류를 중복 출력하지 않는다. 입력창 바로 위의 알림/오류를 클릭하면
화면에서 생략된 부분과 줄바꿈을 포함한 전체 원문을 복사한다. REC 영역은 복사 대상에서 제외한다.
Ctrl+Q로 프로젝트 패널에 포커스를 준 뒤 세션 행에서 r로 alias 편집, Enter 저장, Esc 취소한다. 오류 시
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
## Background tasks

세션의 `/background` 명령은 provider가 보고한 현재 run의 백그라운드 작업을
모달로 표시한다. 실행 중에는 foreground가 idle이어도 기존 상태 줄에 작업 수가
남는다. Claude의 실행 목록/시작/완료 이벤트를 지원하며, Codex 명령 문자열에서
백그라운드 실행을 추정하지는 않는다. 출력 파일 경로는 표시만 하고 읽지 않는다.

작업 정보는 서버가 관리하는 작업별 요약을 한 번 받아 복구한다. 과거 대화를 모두
다운로드하지 않으며 요약 순번 이후의 실시간 이벤트를 합친다. 복구 중이거나 실패하면
모달에 표시된다. 업데이트 시 CLI, manager, 프로젝트 runtime 서버 모두 갱신해야 한다.

## 시작 오류와 설정 신뢰

터미널에서 프로젝트 준비 중 신뢰 확인이 필요하면 전체 화면으로 전환하지 않고
기존 출력 아래에 오류 내용과 한 행의 `Exit`, `Trust` 버튼을 표시한다.
기본 선택은 Exit이며 ←/→ 또는 Tab으로 이동하고 Enter로 선택한다.
긴 오류는 PgUp/PgDn으로 스크롤한다. Trust를 선택해야만 같은 프로젝트 요청을
명시적 신뢰 옵션으로 재시도하며, 이 신뢰는 기존 `--trust-config`와 동일하게 저장된다.
일반 실행 오류는 Exit 버튼으로 닫으며 임의로 명령을 재실행하지 않는다.

`cxz up -x WORKSPACE` 또는 `cxz -x up WORKSPACE`는 오류 복구 화면 없이
즉시 오류를 반환한다. 긴 이름은 `--exit-on-error`다. 비대화형 입력, 리다이렉트된
stderr, `--no-attach`, JSON 출력에서도 오류 화면을 열지 않는다.
기존 `--trust-config`를 명시하면 신뢰 선택 없이 진행한다.
Exit 및 실패의 종료 코드는 0이 아니다.
