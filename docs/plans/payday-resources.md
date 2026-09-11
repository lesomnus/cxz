# payday resource API 전환

사용자 요구: payday 일부 패키지 사용이 아니라 리소스 선언→생성→서버 layer의
프레임워크 모델을 사용한다. 기존 rc.1은 이 요구를 충족한 버전이 아니다.

## 계약

- Project: 작업 공간 등록 리소스. Add/Get/List/Watch는 생성 계약을 사용한다.
  컨테이너 Up/Down/Recreate와 상태 조회는 명시적인 확장 RPC다.
- Session: Project에 속한 영속 대화 리소스. Add는 대화를 생성하고,
  Resume/Send/Reply/Interrupt/Stop/History/Events는 서비스 확장으로 선언한다.
- tenant 없는 `global` 리소스. 사용자 인증은 보류하지만 기존 private UDS와
  프로젝트 capability 경계를 제거하지 않는다. tenant 생략이 공개 접근을 뜻하지 않는다.
- Project=7, Session=8 domain UUID를 사용한다. 기존 runtime ID·vendor ID·저널은
  별도 유지하며 기존 rc.1 데이터를 삭제하거나 ID를 바꿔서 새 대화로 만들지 않는다.
- 일반 Patch/Apply/Erase는 생명주기 규칙을 우회하지 못하게 외부에서 닫는다.
  Docker 작업은 DB transaction 안에 넣지 않는다. 리소스 의도와 실행 결과를 분리한다.

## 구현 순서

1. 버전 고정 payday/ORM 도구, resource proto와 extension, 실제 `pd gen` 산출물.
2. generated ent/Sink/Overlay 기반 server layer, resource metadata 및 기존 journal 연결.
3. CLI/TUI/manager↔project 호출을 resource client로 전환, 구 Sessions 공개 서비스 제거.
4. 기존 데이터 재구성, scope/stale run/재시도, TUI 및 Docker 회귀 검증.

완료 판정은 생성 파일의 존재만이 아니다. 실제 요청이 generated resource service와
서버 layer를 지나야 하며, 기존 CLI 이름을 바꾸는 facade만으로 완료라 하지 않는다.

## 전환 결과

위 1–4를 구현하고 deterministic 통합·race·Docker 재생성·동시 provisioning 복구
검증을 통과했다. CLI/TUI view model은 내부 호환 adapter에 남아 있지만 네트워크에는
생성된 `cxz.v2.ProjectService`와 `cxz.v2.SessionService`만 등록한다.

`ProjectService.Add`는 작업 공간을 등록한다. `Up`은 컨테이너만 준비하고,
`SessionService.Add`는 준비된 Project에 대화를 생성한다. `up` CLI는 이를 조합하며,
이미 실행 중인 세션에는 연결하고 종료된 세션에는 `Resume`을 호출한다.
동시 `up`의 List/Add 또는 Resume 경쟁에서는 재조회로 이미 실행 중인 동일한
agent/model의 세션에 연결한다. 명시적인 `new` 충돌은 자동 연결로 바꾸지 않는다.

저장 계층: payday generated ent의 `resources.db`가 리소스와 audit를 저장한다.
기존 `cxz.db`는 runtime registry/event cache로 유지하고 journal·manifest를
운영 상태의 복구 원천으로 사용한다. 사용자 정의 resource 이름/설명·audit까지
재생성되는 것은 아니므로 전체 state volume을 백업한다.

호환성: rc.1 wire API와 호환되지 않는다. manager와 project runtime을 함께
갱신하되 기존 volume을 지우지 말고 project는 명시적으로 recreate한다.
사용자 인증/roster, HTTP/Web UI, Docker Bake는 이 전환에 추가하지 않았다.
