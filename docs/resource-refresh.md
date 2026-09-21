# 리소스 갱신과 부하

## TUI

- 최초 Session List와 등록 Project List를 받는다. 매초 화면 timer는 목록 RPC를 실행하지 않는다.
- 기존 project/session ID를 payday Watch로 구독한다. 필터는 32개씩 나눈다.
- 변경 알림은 150ms 동안 합치고 목록 갱신은 한 번만 진행한다. 진행 중 추가 알림은 한 번의 후속 갱신으로 합친다.
- 구독 대상이 바뀌면 이전 연결을 취소한다. 구독 세대가 지난 메시지는 무시한다.
- 연결 실패 시 1–30초 backoff, 재접속 시 snapshot을 포함하여 다시 구독한다.
- payday Watch는 이름 붙인 기존 행만 지원한다. 다른 클라이언트의 신규 리소스와 계정/바인딩 변경은
  30초 안전 갱신으로 발견한다. 자기 생성/삭제/이름 변경은 명령 완료 직후 갱신한다.
- Session List는 연관 Project/Account/AuthBinding을 함께 반환한다. 세션마다 별도 Get RPC를 보내지 않는다.
- 자동 갱신은 InspectForeign을 실행하지 않는다. 명시적인 CLI 프로젝트 탐색 경로는 기존 외부 컨테이너 진단을 유지한다.

## 서버

- 초기 registry bootstrap 한 번 이후 Project/Session Get/List/Watch는 저장된 projection을 읽는다.
- Linux 프로젝트 runtime은 inotify로 저널 변경을 감지하여 해당 세션만 갱신한다.
  100ms 동안 파일 이벤트를 합친다. 토큰 출력으로 LastSeq만 바뀌면 목록 변경 알림을 만들지 않는다.
- SIGKILL처럼 마지막 저널 기록이 없는 종료도 발견하도록 알려진 PID의 생존 여부를 2초마다 로컬 검사한다.
  이 검사는 Docker/API 호출이 아니다.
- manager는 프로젝트 Events 스트림을 유지해 전달하고, 전달 전에 수신 이벤트를 캐시에 저장한다.
  기존의 200ms History RPC 및 연결 재생성을 제거했다.
- 로컬 journal Events는 변화가 없으면 파일 메타데이터만 확인하고 전체 저널 replay/DB 조회/status RPC를 건너뛴다.
  backlog는 새 파일 기록을 기다리지 않고 계속 전달한다.
- 서버별 30초 reconcile은 외부 변경/누락 복구용으로 남는다. Linux 외의 runtime은 이 안전망을 사용한다.
- 자동 업데이트와 승인 lease 보호를 위한 10초 activity heartbeat, 계정 quota polling은 별도이며 유지한다.

## 대화 로딩

- 프로젝트 runtime은 세션별로 마지막으로 반영한 JSONL 바이트 위치와 이벤트 순번을 기억한다.
  History/Events와 상태 갱신은 추가된 완전한 레코드만 읽어 SQLite에 반영한다.
  History는 supervisor status RPC를 기다리지 않는다. 세션별 잠금으로 서로 다른 저널 읽기를 분리한다.
- 커서는 DB transaction 성공 후에만 전진한다. 미완성 마지막 레코드는 다음 읽기까지 보류한다.
  daemon 재시작이나 파일 교체·축소 시 전체 검증을 다시 하며, 저널이 DB에 반영된 순번보다
  짧으면 기존처럼 오류를 반환한다. 커서와 상태만 메모리에 보관하고 전체 대화는 보관하지 않는다.
- manager는 연속된 128개 이벤트가 캐시에 있으면 DB에서 즉시 반환한다.
  빈 구간이 있거나 마지막 페이지가 덜 찼으면 runtime을 조회한다. History와 Background는
  프로젝트별 gRPC 연결을 재사용하고 컨테이너/토큰 변경 또는 RPC 실패 시 다시 연결한다.
- 대화 Events 구독 시작은 저장된 세션 정보를 사용하며 전체 프로젝트 상태를 다시 조회하지 않는다.
- TUI는 최근 128개 이벤트를 먼저 표시한다. 과거 스크롤과 밀린 이벤트는 페이지 단위로 받는다.
  백그라운드 작업 복원은 Background RPC 한 번으로 작업별 요약과 순번을 받아 처리하며,
  과거 대화를 모두 다운로드하지 않는다. 이후 이벤트는 요약 순번 이후만 적용한다.
  세션을 전환하면 진행 중인 작업 상태 요청도 취소한다.
- Background RPC가 없는 이전 서버는 업데이트 안내를 표시한다. 전체 조회로 되돌아가지 않는다.
  이 변경은 CLI, manager, 프로젝트 runtime 모두 갱신해야 적용된다.
- 녹화의 `history_rpc`는 초기/과거/밀린 기록/작업 상태 요청 시간, protobuf 응답 바이트 수,
  이벤트 수와 오류 코드를 기록한다. `history_first_render`는 초기 요청 시작부터 첫 페이지의
  TUI 렌더링 완료까지이며 실제 터미널 표시 시간은 아니다. 본문은 기록하지 않는다.

## 검증과 적용

실제 in-process gRPC 테스트에서 반복 목록 조회의 runtime snapshot은 최초 1회, Session List의
클라이언트 RPC는 1회(추가 relation Get 없음), 변화 없는 Watch에서 추가 RPC/동기화 없음,
변경 알림과 취소를 검증했다. 통합 테스트는 승인/중단/daemon 재시작/프로세스 강제 종료/저널 복구를 포함한다.

CLI만 바꾸면 서버 쪽 부하는 남는다. 새 CLI로 manager를 업데이트하고 프로젝트 runtime 서버도
갱신해야 전체 경로에 적용된다. 프로젝트 재생성을 선택할 때는 writable layer 삭제 경고를 확인한다.
