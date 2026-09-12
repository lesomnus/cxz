# Agent Accounts

Account는 cxz 사용자 인증이나 tenant가 아니라 Claude/Codex 구독 로그인 프로필이다.
`Project → Session → Account`로 연결하고, 같은 Project의 여러 대화가 서로 다른
Account를 사용할 수 있다. 한 프로젝트에 활성 세션 하나라는 기존 제약은 유지한다.

```sh
cxz account add --agent codex --name "Personal" personal-codex
cxz account add --agent codex --name "Company" work-codex
cxz account login personal-codex
cxz account login work-codex
cxz account status work-codex
cxz new --account work-codex .
cxz up .                         # 기존 세션의 Account 유지
cxz account get work-codex
```

- payday `AccountService.Add/Get/List/Watch`, 전역 unique alias, domain 9.
  Account alias/agent와 Session.account는 변경할 수 없다. 삭제도 현재 닫혀 있다.
- `SessionService.Add`는 AccountRef를 받는다. 등록되지 않은 계정, vendor 불일치,
  인증 파일 누락을 거부한다. TUI Ctrl+N에서 Tab으로 Account를 고르고 경로를 입력한다.
- 로그인은 설치된 manager에서 실행하며 사용자가 직접 vendor 인증을 완료해야 한다.
  취소/실패한 로그인은 기존 인증을 덮어쓰지 않는다. `status`는 파일 형식/존재만
  검사하며 유효한 구독·네트워크 인증 성공을 보장하지 않는다.
- manager의 `accounts/ALIAS/config`에 인증 원본을 0600으로 보관한다. SQLite에는
  메타데이터만 저장한다. 토큰은 API 응답, payday audit, journal에 넣지 않는다.
  프로젝트로의 전달은 소유권 확인 후 `docker exec` stdin을 이용한다.
- 프로젝트의 `accounts/ALIAS/config`와 `accounts/ALIAS/home`를 선택한다.
  HOME/XDG 경로를 분리하고 inherited OpenAI/Anthropic/Claude/Codex 및 cloud 인증
  환경변수를 제거한다. Codex는 OpenAI provider와 file credential store를 명시한다.
- manager 로그인 변경은 다음 새 세션/정지된 세션 resume 때 반영한다. 동일 로그인
  원본을 다시 전달할 때 프로젝트에서 vendor가 갱신한 토큰을 덮어쓰지 않는다.
  프로젝트의 갱신 토큰을 manager나 다른 프로젝트로 역동기화하지 않는다. 여러
  프로젝트에서 같은 Account를 쓰다가 vendor가 refresh token을 무효화하면 재로그인이
  필요할 수 있다. 다른 Account로 자동 전환하지 않는다.
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
