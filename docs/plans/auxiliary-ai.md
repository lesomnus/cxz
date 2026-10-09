# 보조 AI 작업: 인증과 제한된 세션 문맥

상태: 초기 구현 포함. 사용 방법과 현재의 구체적인 제한은
[보조 AI 작업](../auxiliary-ai.md)을 참고한다. 아래는 설계 배경과 검증 방향이다.
추적: [#27](https://github.com/lesomnus/cxz/issues/27).
공유 메모리 compact는 [#36](https://github.com/lesomnus/cxz/issues/36)에서 별도로 구현한다.
전송 표면은 [보조 AI 작업 API](auxiliary-job-api.md)에서 따로 설계한다.

## 사용자 경험

연결된 Manager의 Settings에서 작업별로 활성화 여부, Account, model, effort를
선택한다. Agent 종류와 인증 backend는 Account를 따른다. 초기 작업은 대화 요약과
다음 입력 추천이며, 기본값은 모두 비활성이다. 설정을 바꿔도 작업 중인 원본
에이전트를 재시작하지 않는다.

요약은 목적, 현재 상태, 남은 일, 막힌 일을 개조식 항목 5개 이내로 적는다. 응답
문장을 옮겨 적지 않고 자기 표현으로 압축해, 다시 읽지 않고 훑어볼 수 있게 한다.
다음 입력은 사용자가 그대로 보낼 수 있는 명령형 한 줄로 제안하되, 할 일이 명확하지
않으면 비워 둔다.
최근 사용자 메시지를 기준으로 언어를 맞춘다. 결과는 AI가 만든 제안임을 표시하고,
입력창에 이미 작성 중인 텍스트를 자동으로 바꾸거나 추천을 자동 전송하지 않는다.

계정 선택과 로그인 완료는 별개다. 설정 화면은 `로그인 필요`, `사용 가능`, `실행 중`,
`오류`를 구분하며, 지원하지 않는 model/effort나 인증 만료를 다른 계정으로 우회하지 않는다.

## 인증 프로필과 작업 문맥의 수명

| 데이터 | 분리 단위 | 수명 |
| --- | --- | --- |
| 보조 작업 인증 프로필 | Manager × Account | 작업 사이에 유지; 명시적 재로그인/삭제로 교체 |
| 요약용 상태와 최근 대화 | Manager × Project × 원본 Session | 크기 제한 내에서 유지; 다른 세션과 합치지 않음 |
| 모델 실행 | 작업 ID | 성공·실패·취소·timeout 시 종료 |
| 표시 결과 | 원본 Run/Turn와 설정 revision | 해당 입력에 대한 결과로만 사용 |

같은 Account로 요약과 추천을 수행해도 서로 다른 원본 세션의 문맥을 하나의 provider
대화에 쌓지 않는다. 각 작업은 새 provider 대화에서 시작하고, cxz가 필요한 문맥만
구성해서 넘긴다. 인증 프로필을 재사용하는 것과 대화 전체를 재사용하는 것은 별개다.

## 인증

### Codex 중앙 인증

기존 `brokered-access-token` Account는 중앙 로그인을 재사용한다. Manager가 보조
실행기에 전용 binding과 capability를 제공하고, 실행기는 token broker를 통해
access token을 공급받는다. 원본 프로젝트의 capability를 가져다 쓰지 않는다.

여기서 capability는 cxz가 생성하는 예측 불가능한 비밀값이다. broker가 그 값을
확인해 어떤 Account의 토큰을 공급할지 결정한다. 공급자에게 새로운 OAuth 권한을
발급받거나, 공급자 토큰을 '요약만 가능한 토큰'으로 바꾸는 것이 아니다. 작업 종류,
도구 차단, 입출력 크기와 실행 시간은 cxz 실행기가 제한한다.

현재 `accounts.IssueGrant`와 `ResolveBinding`은 Project × Account를 전제로 한다.
auxiliary 범위를 명시적으로 모델링하고, 발급·설치·조회·폐기에서 같은
범위를 검증한다. 임의의 프로젝트 이름으로 보조 작업을 위장하지 않는다. 토큰과
capability는 설정 응답, 작업 기록, provider 출력, 명령행 인자에 포함하지 않는다.
중앙 credential과 refresh 소유권은 기존 broker에 남긴다.

### 로컬 OAuth 인증

Claude는 Account별 보조 작업 전용 프로필에서 최초 한 번 공식 로그인 절차를
완료한다. 기존 세션에서 사용 중인 credential 파일을 복사하거나 그 프로필을
같이 사용하지 않는다. 같은 구독 계정으로 로그인할 수 있지만 프로필은 독립적이다.

명시적으로 `project-local-oauth`를 선택한 Codex Account도 중앙 인증으로 몰래
전환하지 않는다. 같은 전용 프로필 로그인 규칙을 적용한다. 모든 backend에서
기존 Account의 agent/backend 설정을 그대로 존중한다.

요약·추천과 향후 메모리 compact는 같은 보조 인증 프로필을 사용할 수 있다.
초기 구현에서는 Account별 작업을 직렬 실행한다. 로그인과 refresh도 실행 중인
프로필과 경쟁하지 않도록 같은 소유권/잠금 규칙을 따른다. 작업이 끝나면 provider
프로세스를 종료하고 프로필 잠금을 해제한다.

## 짧은 문맥으로 품질 유지

최종 응답 하나만 전달하면 사용자의 제약이나 앞선 결정을 놓칠 수 있다. 대신 다음
네 부분을 조합한다.

1. 고정 지시문과 출력 형식.
2. 원본 세션의 상태 checkpoint: 목적, 제약, 결정, 완료, 남은 일, 제외한 방향.
3. 최근 사용자 메시지와 에이전트 최종 응답 원문.
4. 이번 턴의 사용자 입력과 최종 응답.

중간 도구 호출, stdout 전체, reasoning은 기본 입력에서 제외한다. 이전 checkpoint와
모든 발췌문에는 원본 Turn 식별자를 연결하고, 잘리거나 이미 보존 정책으로 삭제된
내용은 누락 사실을 표시한다. 새 기능을 켰다고 모든 과거 대화를 모델에 보내지는 않는다.

최근 사용자 수정·금지 사항은 가능한 한 원문으로 유지한다. 오래된 checkpoint와
충돌하면 최신 사용자 발언을 우선한다. 요청과 계획을 완료된 사실로 바꾸지 않으며,
확인할 근거가 부족하면 추천을 생략하거나 불확실성을 표시한다.

문맥은 기간이 아니라 예산으로 자른다. 초기 측정 출발점은 checkpoint 약 1–2k,
최근 대화 약 4–6k tokens이며, 실제 기본값은 한국어·영어와 model별 측정 후 정한다.
현재 턴, 고정 지시문, 출력 예약 공간을 포함한 전체 한도와 메시지별 한도도 둔다.
tokenizer가 없는 경우 추정치임을 명시하고 UTF-8 byte 상한을 별도로 적용한다.

checkpoint는 매 턴 다시 만들지 않는다. 최근 대화가 예산을 넘을 때 기존 checkpoint와
밀려나는 완료 턴을 압축하고, 최신 턴은 원문으로 남긴다. 갱신 실패 시 기존 checkpoint를
보존하며 한도를 초과한 입력을 그대로 전송하지 않는다. 원본 revision을 확인한 뒤
성공한 후보만 교체한다. 이 내부 문맥 정리는 #36의 공유 메모리 파일 편집과 다르다.

작업별 계정이 다르면 각 실행에는 선택된 계정만 사용한다. checkpoint 생성에 쓰는
프로필은 명시적으로 결정해야 한다. 초기안은 활성화된 요약 프로필, 요약이 꺼져 있으면
추천 프로필을 사용하는 것이며, 실행 전 Settings에 이 관계를 표시한다. 계정·model
전환 시 이미 큐에 든 작업은 새 계정으로 재라우팅하지 않는다.

문맥을 줄이면 입력량을 제한할 수 있지만, 구독 사용량의 실제 절감률은 실행 측정으로
확인해야 한다. provider의 prompt cache 적중이나 비용 0을 전제로 설계하지 않는다.

## 실행·결과 관리

Manager가 설정과 작업 수명, 중복 방지, 세션별 checkpoint를 소유한다. Client는
표시와 명시적인 추천 적용만 담당한다. 여러 클라이언트가 같은 턴을 보고 있어도
한 번만 생성한다. 클라이언트 연결 종료가 원본 세션에 영향을 주지 않는다.

실행 위치는 Manager가 관리하는 별도 helper 컨테이너다. 프로젝트 workspace,
프로젝트 상태 볼륨과 Docker socket을 마운트하지 않고, 필요한 텍스트와 해당 Account의
보조 인증만 전달한다. provider 실행 파일은 기존 distribution 캐시로 공급하고 로그인은 Manager 호스트의
CLI 표준 입출력으로 연결한다. 이 컨테이너를 Session supervisor와 혼동하지 않는다.

초기 실행기는 text-only다. provider의 도구와 MCP를 비활성화하고, 예상치 못한 도구·승인
요청은 거부한다. 지시문에 '도구를 쓰지 말라'고 적는 것만으로 제한을 구현하지 않는다.
새 대화에 프로젝트 지시문·skills·MCP 설정이 자동 주입되지 않는지도 검증한다.

작업 키는 원본 Project/Session/Run/Turn, 입력 revision, 작업 설정 revision으로
구성한다. 요약과 추천의 Account/model/effort가 같으면 한 번의 구조화된 응답으로
둘을 생성한다. 다르면 각 설정대로 실행한다. 설정이 바뀌거나 새 입력이 시작되면
오래된 추천을 적용할 수 없도록 하고, 실행 중 작업은 취소하거나 결과를 폐기한다.

실행마다 timeout, 입력·출력 한도, 취소, Account별 동시성 1과 전체 큐 상한을 둔다.
실패를 숨기거나 무한 재시도하지 않는다. Manager 재시작으로 끊긴 작업은 interrupted로
처리하고, 유료 모델 호출이 완료됐는지 불명확한 작업을 자동 재전송하지 않는다.
설정과 결과의 보존량도 제한하며, 세션 삭제 시 파생 문맥·결과를 정리한다.

원본 대화의 turn metric에 보조 사용량을 합치지 않는다. 공급자가 반환한 사용량을
별도 표시하고, 제공하지 않은 token 수나 비용은 추측해서 채우지 않는다.

## 구현 순서와 검증

1. Account별 보조 binding/profile, 로그인 상태 API, 로그인·refresh 잠금과 취소.
2. 도구 없는 provider adapter와 제한된 실행기. 실제 model/effort 목록으로 검증.
3. 세션별 문맥 저장, checkpoint 교체, 작업 ID와 중복 방지·취소·재시작 처리.
4. Settings 매핑, 요약·추천 표시, 명시적 적용과 오래된 결과 차단.
5. 동일 입력으로 지연·사용량·언어·정확성을 측정해 문맥 예산 기본값 결정.

테스트는 fake provider와 임시 프로필로 인증 범위 혼동, 잠금 충돌, 취소와 timeout,
지원하지 않는 effort, 삭제된 Account, 여러 클라이언트의 중복 요청, 세션 간 문맥 혼합,
checkpoint 교체 충돌과 실패, Manager 재시작, 사용자 입력 보존을 검증한다.
실제 구독 실행은 별도 수동 검증으로 구분한다.

참고할 현재 구현:

- `internal/accounts/backend.go`, `session.go`: backend와 세션별 인증 프로필.
- `internal/accounts/broker.go`: capability, 중앙 인증과 token 공급.
- `internal/supervisor/codex.go`, `supervisor.go`: 기존 provider 프로토콜.
- `internal/agentview/models.go`: model/effort capability 해석.
- `internal/workspace/history.go`: Manager 이벤트 캐시와 보존 경계.
- `internal/tui/settings_page.go`: 연결된 Manager의 설정 화면.

이 파일들은 재사용할 경계의 참고 자료다. 기존 Supervisor의 프로젝트 접근 권한을
그대로 보조 실행기에 넘기지는 않는다.

## 초기 구현에서 확정한 사항

- Manager가 helper 컨테이너와 5초 간격 수집을 관리한다. helper에는 전용 인증
  volume과 읽기 전용 tools volume만 전달한다.
- 전용 로그인은 Manager 호스트의 `cxz ai login`에서 공식 CLI의 표준 입출력을
  연결한다. Windows 원격 클라이언트의 로그인은 호스트에서 수행한다.
- Settings에서 Account/model/effort를 검증한 뒤 활성화한다. checkpoint는 활성화된
  요약 프로필, 없으면 추천 프로필을 따른다.
- 현재 예산은 추정 token 값 대신 엄격한 byte 상한이다. 구독을 사용한 지연·품질
  측정과 기본값 최적화는 아직 수행하지 않았다.
- 요약은 응답 바로 아래, 추천은 빈 입력창의 ghost로 표시한다. `/summary on/off`,
  `/suggest on/off`는 세션별 자동 설정이고, 인자 없는 명령은 꺼져 있을 때만 1회 생성한다.
  Alt+G로 추천을 적용하기 직전 서버 상태와 로컬 초안을 다시 확인한다.
