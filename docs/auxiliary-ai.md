# 보조 AI 작업

Settings → **AI tasks**에서 요약(Summary)과 다음 입력 추천(Next-message suggestion)을
각각 켜고 Account, model, effort를 지정한다. 기본값은 모두 꺼짐이다. Enter로 편집,
Tab으로 필드 이동, Ctrl+L로 해당 계정의 모델·effort 목록 조회,
Ctrl+S로 검증 후 활성화, Space로 활성화/비활성화를 전환한다.

원본 에이전트의 계정·모델과 별도로 선택할 수 있다. 지원하지 않는 모델이나 effort,
인증 실패는 오류로 표시하며 다른 계정으로 바꾸지 않는다. 최초 사용 시 Manager가
공식 에이전트 CLI를 준비하므로 모델 목록 조회에 시간이 걸릴 수 있다.

## 인증

- 중앙 Codex Account는 기존 `cxz account login ACCOUNT` 로그인을 재사용한다.
  프로젝트와 구별되는 auxiliary binding/capability로 토큰을 공급한다.
- Claude와 `project-local-oauth` Codex Account는 **Manager 호스트**에서
  `cxz ai login ACCOUNT`로 보조 작업 전용 로그인을 완료한다. 원본 세션 인증을
  복사하지 않는다. 같은 구독 계정을 사용해도 보조 프로필은 독립적이다.
- `cxz ai login ACCOUNT`는 중앙 Codex Account인 경우 중앙 로그인 명령으로 연결한다.
  Windows에서 SSH remote를 사용하는 경우 로그인은 연결된 Linux 호스트에서 실행한다.
  설정·상태·모델 조회와 결과 보기는 Windows TUI에서도 사용할 수 있다.

프로필은 Manager 설치에 속한 Account별 전용 Docker volume에 보관한다. 인증은
작업 사이에 유지하며 같은 Account의 작업은 직렬 실행한다. 로그인과 모델 실행은
같은 프로필 잠금을 사용하므로 실행 중 재로그인은 busy 오류로 거부될 수 있다.

## 결과

Manager가 활성화 이후의 새 사용자 입력과 완료 턴을 수집한다. TUI 연결이 끊겨도
수집은 계속되며, 여러 클라이언트가 같은 턴을 보아도 중복 생성하지 않는다.
활성화 이전 대화의 일괄 생성은 하지 않는다. 처음 발견한 세션은 최근 512개 이벤트에서
시작하므로 이미 오래 실행 중인 턴의 입력이 범위 밖이면 그 다음 턴부터 처리한다.

- `/summary`: 최신 결과, 상태, 다음 입력 추천과 별도 사용량을 표시한다.
- 입력창이 비어 있으면 현재 추천을 힌트로 표시한다.
- **Alt+G** 또는 `/suggest`: 서버에서 결과가 아직 유효한지 확인하고 입력창에 복사한다.
  사용자가 검토하고 직접 전송해야 한다. Alt+Enter의 기존 줄바꿈 동작은 유지한다.

새 입력, 다른 Run, 설정 변경으로 오래된 추천은 적용할 수 없게 된다. 조회 중 사용자가
입력한 초안도 덮어쓰지 않는다. 추천할 다음 작업이 없으면 추천은 빈 값일 수 있다.
요약과 추천의 계정·모델·effort가 같으면 한 번의 호출로 생성한다.

## 문맥과 제한

매 작업은 새 provider 대화로 시작한다. 원본 Session별 checkpoint와 최근 사용자
입력·에이전트 최종 응답을 cxz가 구성해 전달한다. 도구 결과와 reasoning은 제외하고,
도구 실행 전의 중간 설명도 제외한다. 원본 에이전트의 대화·메모리는 수정하지 않는다.

현재는 tokenizer 없이 **UTF-8 byte 상한**으로 제한한다. token 수나 구독 비용
절감률을 보장하는 값은 아니다.

| 항목 | 현재 제한 |
| --- | --- |
| 사용자 입력 / 최종 응답 | 각각 8 KiB, 초과 시 누락 표시 |
| 최근 턴의 checkpoint 갱신 기준 | JSON 20 KiB 또는 32턴 초과 |
| checkpoint | 6 KiB |
| 모델에 전달할 대화 문맥 | 32 KiB + 고정 지시문 |
| 구조화된 모델 응답 | 12 KiB |
| 보관하는 최근 턴 | JSON 48 KiB / 64턴, 실패가 반복되어도 무한 증가하지 않음 |
| 파생 상태 보관 | 최대 256세션, 오래 사용하지 않은 것부터 정리 |
| 실행 | 최대 4개 작업, Account별 직렬, 실행 중/대기 중 세션 최대 32개 |
| 시간 | 작업당 최대 3분; 전용 로그인 최대 15분 |

checkpoint는 예산을 넘을 때만 갱신하고, 실패 시 기존 checkpoint를 유지한다.
요약이 활성화되어 있으면 요약 프로필, 아니면 추천 프로필로 갱신한다. 저장 한도로
이전 턴이 제외되면 문맥이 불완전하다는 안내를 모델에 전달한다. 긴 한국어 대화 등에서
실제 지연·품질·사용량을 측정하며 이 기본값을 조정할 수 있다.

실행기는 전용 helper 컨테이너를 사용한다. 원본 workspace·프로젝트 상태와 Docker
socket을 마운트하지 않는다. 도구·MCP를 비활성화하고 예상하지 못한 요청은 거부한다.
소스 세션의 실행은 기다리지 않는다. 실패를 자동 반복하거나 Manager 재시작 후 완료
여부가 불명확한 유료 호출을 재전송하지 않는다.

사용량은 공급자가 알려준 값만 별도로 표시한다. 원본 turn metric에 합산하지 않는다.
원본 세션을 삭제하면 파생 문맥·결과도 정리한다. Account의 전용 로그인 프로필은
다른 세션의 보조 작업에서도 쓰므로 세션 삭제와 함께 지우지 않는다.

## CLI

```sh
cxz ai list
cxz ai models work
cxz ai login work
cxz ai set summary --account work --model MODEL --effort EFFORT
cxz ai set suggestion --account work --model MODEL --effort EFFORT
cxz ai disable suggestion
cxz ai status SESSION_ID
cxz ai cancel SESSION_ID
```

model/effort는 `cxz ai models`에서 확인한 값을 사용한다. effort를 생략하면 공급자의
기본값을 사용한다. `cancel`은 보조 작업만 취소한다.

공유 메모리 compact는 [#36](https://github.com/lesomnus/cxz/issues/36)의 별도 기능이며
이번 기능에 포함되지 않는다. 인증·문맥의 설계 배경은
[보조 AI 작업 설계](plans/auxiliary-ai.md)를 참고한다.
