# Agent Accounts

Account는 cxz 사용자 인증이나 tenant가 아니라 Claude/Codex 구독 로그인 프로필이다.
`Project → Session → Account`로 연결하고, 같은 Project의 여러 대화가 서로 다른
Account를 사용할 수 있다. 한 프로젝트에 활성 세션 하나라는 기존 제약은 유지한다.

## AgentKind / AuthBackend / AuthBinding

- `AgentKind`: `claude`, `codex`. 모델 서비스 provider와 구별한다. API의 `agent`
  문자열은 등록된 AgentKind로 검증한다.
- `AuthBackend`: 로그인·인증 확인·실행 환경·갱신 주체를 결정하는 전략이다.
  `Account.auth_backend`에 고정한다. 생략 시 Account 생성 시에만 agent 기본값을
  선택하며, 세션 실행/복구에서는 빈 값이나 알 수 없는 값을 기본값으로 대체하지 않는다.
- `AuthBinding`: Account와 실제 인증 범위의 연결이다. payday domain 10 리소스로
  Account, 선택적 Project, backend, scope, 비밀 저장소 참조를 저장한다. 현재 구현은
  project scope만 허용한다. binding의 존재는 로그인 성공을 의미하지 않는다.
- `Session.auth_binding`도 생성 후 고정한다. backend/binding ID는 runtime manifest와
  manager 캐시에 보존하며, DB를 재구성해도 같은 연결을 복원한다. 다른 프로젝트·계정의
  binding으로 실행할 수 없다. 이전 개발 데이터에 없는 backend/binding은 임의 배정하지 않는다.

에이전트별 registry는 기본 backend와 지원 backend factory 목록을 가진다. Backend는
`Info`, `Binding`, `Login`, `Check`, `Launch`를 구현한다. CLI는 backend가 선언한
workflow로 진입하고, supervisor는 backend가 반환한 환경변수·인자를 사용한다.
token을 직접 반환하는 공통 API로 모든 공급 방식을 억지로 맞추지 않는다.

| AgentKind | 기본 / 활성 backend | 범위 | 로그인 workflow | 갱신 담당 |
|---|---|---|---|---|
| claude | project-local-oauth | Project × Account | 프로젝트에서 Claude 로그인 | Claude |
| codex | project-local-oauth | Project × Account | 프로젝트에서 Codex device login | Codex |

`brokered-access-token`, `api-key`는 아직 구현·등록하지 않았다. 선택하면 명시적으로
실패한다. 다른 backend나 계정으로 자동 전환하지 않는다. 중앙 공급을 추가할 때는
backend 구현과 에이전트별 등록, 해당 workflow와 갱신 통합 검증이 필요하다.

```sh
cxz account add --agent codex --name "Personal" personal-codex
cxz account add --agent codex --name "Company" work-codex
cxz account login personal-codex
cxz account login --project . work-codex
cxz account status --project . work-codex
cxz new --account work-codex .
cxz up .                         # 기존 세션의 Account 유지
cxz account get work-codex
cxz account backends              # 설치 없이 지원 매핑 확인
cxz account bindings work-codex    # 비밀 없는 인증 연결 목록
cxz account add --agent claude --auth-backend project-local-oauth work-claude
```

- payday `AccountService.Add/Get/List/Watch`, 전역 unique alias, domain 9.
  Account alias/agent/backend와 Session.account/auth_binding은 변경할 수 없다.
  AuthBinding도 `Add/Get/List/Watch`를 사용하며 Add 재시도는 같은 binding으로 수렴한다.
  일반 Patch/Apply/Erase는 닫혀 있다. credential_ref는 backend가 만들며 호출자가 지정할 수 없다.
- `SessionService.Add`는 AccountRef와 AuthBindingRef를 받는다. CLI/TUI가 선택된
  Account와 Project의 binding을 해결해 전달한다. 등록되지 않은 계정, vendor 불일치,
  인증 파일 누락을 거부한다. TUI Ctrl+N에서 Tab으로 Account를 고르고 경로를 입력한다.
- 로그인은 지정 프로젝트에서 실행하며 사용자가 직접 vendor 인증을 완료해야 한다.
  `--project` 생략 시 현재 디렉터리다. login은 필요한 컨테이너·에이전트를 준비하지만
  세션은 생성하지 않는다. 같은 Account도 다른 Project에서는 독립 로그인이 필요하다.
  취소/실패한 로그인은 기존 인증을 덮어쓰지 않는다. `status`는 파일 형식/존재만
  검사하며 유효한 구독·네트워크 인증 성공을 보장하지 않는다.
- 프로젝트의 `accounts/ALIAS/config`에 인증 원본을 0600으로 보관한다. SQLite에는
  메타데이터만 저장한다. 토큰은 API 응답, payday audit, journal에 넣지 않는다.
  manager는 인증 파일을 보관하거나 다른 프로젝트로 전달하지 않는다.
- 프로젝트의 `accounts/ALIAS/config`와 `accounts/ALIAS/home`를 선택한다.
  HOME/XDG 경로를 분리하고 inherited OpenAI/Anthropic/Claude/Codex 및 cloud 인증
  환경변수를 제거한다. Codex는 OpenAI provider와 file credential store를 명시한다.
- 재로그인 전 활성 세션을 중단해야 한다. vendor가 갱신한 토큰은 해당 프로젝트·계정에
  그대로 남으며 다른 프로젝트와 동기화하지 않는다. 이는 기존 아키텍처 §4.2의
  refresh-token 회전 충돌 방지 원칙을 유지한다. Codex의 중앙 단기 토큰 공급은
  아직 구현하지 않았으며 Claude와 동일하게 프로젝트별 독립 구독 로그인을 사용한다.
- 컨테이너 재생성은 project volume의 Account별 인증·대화 파일과 manifest의 계정
  연결을 유지한다. manager와 project의 **전체 state volume**을 비공개로 백업한다.
  기존 account 없는 개발 데이터는 자동 계정 할당/인증 이관하지 않는다.

## 격리 범위

이 기능은 **잘못된 계정의 자동 사용을 방지하는 인증 프로필 격리**다. 서로 신뢰하지
않는 사용자/코드를 위한 보안 경계는 아니다. 같은 프로젝트의 과거 Account 디렉터리와
작업 파일은 같은 OS 사용자에게 접근 가능하다. 파일·Git·과거 인증까지 개인/회사 간
분리해야 한다면 Project/volume도 분리해야 한다. 임의 작업 코드에 회사 인증을 맡기는
문제는 승인 정책과 workspace 신뢰 설정으로 별도로 관리한다.

현재 지원 범위는 Claude/Codex 구독 OAuth 로그인이다. API key, Bedrock/Vertex/Azure,
cxz 사용자 인증/roster, Project 기본·허용 Account 정책은 이번 범위에 포함하지 않는다.
프로필 이름은 실제 vendor 조직을 자동 증명하지 않는다. 로그인 화면에서 개인/회사
계정을 직접 확인해야 한다. 실계정 로그인과 유료 대화 검증은 사용자 인증 이후 수행한다.
