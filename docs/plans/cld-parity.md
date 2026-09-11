# cld 사용 경험을 계승하는 cxz 1단계

2026-09-11. 이전 TUI 구현은 기반 구현이며 이 단계의 완료를 의미하지 않는다.
판단 우선순위: 사용자 지시 → cxz architecture의 결정 → cld의 사용 경험.

## 완료 조건

- `cxz install`: Docker 관리 서버를 백그라운드 설치, 준비 확인, 재설치/제거.
  호스트에는 Docker/cxz만 필요. 원격 엔진은 명시적인 공유 경로 매핑을 사용한다.
- `cxz up/new [path]`: devcontainer 설정 발견/선택/기본 생성, 공식 devcontainer
  CLI로 생성, agent/state/tools와 supervisor를 생성 시점에 제공, 준비 후 TUI 연결.
- Claude/Codex 선택과 저장. `up`은 기존 세션 재접속, `new`는 새 대화.
  한 작업 트리의 활성 agent는 하나로 유지하며 다른 vendor로 변경 시 명시적으로 중단한다.
- `ls`, `attach/it`, `watch/tui`, `stop`, `down`, `up`, `recreate`를 연결한다.
  컨테이너 내부 클라이언트는 그 프로젝트 세션만 접근한다.
- daemon 재시작에 agent가 영향받지 않고, 프로젝트 컨테이너 재생성 시 volume에
  보존한 실제 vendor 대화와 cxz journal로 resume한다. 이전 입력을 자동 재전송하지 않는다.
- VS Code 등 외부 생성 컨테이너는 감지/표시만. 일반 up은 삭제하지 않는다.
  명시적 recreate 전에 정확한 대상과 writable layer 손실/편집기 연결 해제를 표시한다.
- 기존 devcontainer 설정을 존중하고, 불필요한 privilege/Docker socket 접근은
  프로젝트에 주지 않는다. Compose sidecar의 소유권도 확인한다.
- 단위/통합 테스트, 실제 Docker 흐름, 실제 Claude/Codex 프로토콜 검증.
  계정 조작이 필요한 최초 로그인은 사용자 수행 단계로 남긴다.

## 구현 순서 및 진행

1. 구현: agent 종류를 명시하는 API, Codex app-server adapter, 프로토콜 단위 테스트.
2. 구현: 관리 이미지/installer, 소유 프로젝트 provisioning, scoped 프로젝트 runtime.
   실제 Docker에서 install → up → Claude/Codex 세션 준비 성공. 기존 hook도 실행됨.
3. 구현: CLI/TUI 자동 연결, agent 선택, 프로젝트 내부 client, login/shell/exec,
   foreign 거부/명시적 recreate, down 전 history 수집, 관리 서버 교체 시 network 재연결.
4. 검증 중: 실제 Docker image/Alpine/Compose 성공. Compose sidecar 통신,
   tools read-only, 내부 프로젝트 범위 확인. 실제 vendor 대화/승인/중단/반복 복구
   probe를 추가하고 실행 중. 최초 인증은 별도 사용자 단계이며 probe는 기존
   access token의 일회성 투영만 사용하고 refresh token은 복사하지 않는다.

실제 검증에서 수정한 사항: Compose의 named volume 접두사/readonly 변환,
기본 sidecar network 누락, 관리 컨테이너 교체 후 project network 재연결,
비 TTY 표준입력 판별, Codex의 첫 turn 이전 비영속 빈 thread 처리.

웹 UI·roster 사용자 인증·호스팅 IDE·원격 공개는 이 TUI 단계와 분리한다.
이는 cld의 외부 컨테이너 편입 방식이나 tmux 구현을 그대로 복제하는 작업이 아니다.
