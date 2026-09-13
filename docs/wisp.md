# Wisp

`cxz wisp`는 컨테이너 remote user 권한으로 실행하는 내부 workspace helper다.
첫 적용 범위는 읽기 전용 경로 탐색이다. 에이전트 supervisor, 인증, 대화, 터미널 PTY는 변경하지 않는다.

## 연결과 수명

- TUI는 이미 받은 프로젝트 메타데이터를 이용한다. 경로 탐색 중 전체 프로젝트 조회나 reconcile은 하지 않는다.
- 최초 연결에서 실행 상태와 `cxz.project`/`cxz.owner` label을 검증한다.
- `docker exec -i --user USER CONTAINER /cxz/tools/cxz wisp`를 한 번 열고 stdio 연결을 재사용한다.
- 한 TUI 안에서 프로젝트/컨테이너/사용자별 helper 하나를 공유한다. 여러 TUI 간 singleton 데몬은 아니다.
- 컨테이너 식별자가 바뀌면 이전 helper를 종료한다. 종료된 연결은 다음 조회에서 다시 연다.
- TUI 종료 시 연결을 닫는다. 포트 공개, root 승격, credential 전달은 없다.

## 내부 프로토콜

JSON Lines로 hello(protocol version), 경로 요청, entry 응답, done/error를 전달한다.
공개 payday 리소스 API를 대체하는 API가 아니라 TUI와 컨테이너 사이 전용 transport다.
stdout은 프로토콜 전용이며 stderr를 경로 데이터로 해석하지 않는다.

동일 연결의 요청은 직렬화한다. 입력 변경으로 취소한 요청은 소비자에게 전달하지 않고
done까지 drain하여 다음 요청에 응답이 섞이지 않게 한다. 조회 deadline이 지나면 연결을 종료한다.
컨테이너 측은 디렉터리 항목 2,048개로 작업량을 제한하며 파일 내용은 읽지 않는다.

Go `ReadDir`를 작은 batch로 읽고 파일 종류, executable mode bits, 링크 대상을 전달한다.
셸 실행/명령 보간/fzf 설치는 없다. 현재 경로의 fuzzy 검색은 TUI에서 수행한다.
디렉터리를 다시 열면 재조회한다. 별도 TTL 캐시는 아직 두지 않는다.

## 검증

- 동일 연결 반복 조회, 취소 후 응답 격리, 한글/공백/특수문자, executable/깨진 링크, 잘못된 경로.
- 실제 Docker fixture에서 비-root 권한 거부, 소유권 거부, 프로세스 종료 후 재연결.
- 로컬 Docker fixture 측정: 기존 inspect+exec 약 63ms, wisp 최초 약 68ms,
  재사용 조회 약 0.34–0.36ms. 전체 프로젝트 RPC와 TUI debounce/렌더링을 제외한 측정이며
  사용자 환경의 성능을 보장하는 수치는 아니다.

기존 컨테이너는 공유 도구 바이너리 업데이트가 필요하다. 없는 명령을 무한 재시도하거나
느린 전체 프로젝트 조회 방식으로 조용히 fallback하지 않고 업데이트 안내를 표시한다.
