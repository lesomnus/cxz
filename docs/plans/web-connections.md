# 연결: 브라우저가 어디에 붙어 있는지

상태: 1~4단계 구현됨 (계약, 서버의 거절, 브라우저 구현, UI 자리). 5·6단계는 남아 있다. 웹 클라이언트에는 연결이라는 개념이 없고, 이 문서는 그것을 리소스 계약
하나로 세우고 클라이언트마다 구현하는 길을 적는다. 원격 접속 자체는
[원격 접속](../remote.md)에 이미 문서화돼 있다.

## 연결은 서버의 개념이 아니다

이 점을 먼저 박아둔다. 연결은 **클라이언트가 어디에 붙을지에 대한 사실**이고,
매니저는 자기가 어떤 이름으로 불렸는지 모른다. `cxz connection ls`가 그 규칙을
그대로 보여준다 — 로컬 `settings.jsonc`를 읽고, **어떤 서버에도 접속하지 않으며**,
베어러 토큰 파일을 읽지 않고 자격증명을 표시하지 않는다.

그래서 이 계약은 서버가 제공하는 데이터가 아니라, **클라이언트가 구현하는 질문**이다.

| 클라이언트 | `ConnectionService.List`의 구현 |
| --- | --- |
| TUI/CLI | 로컬 `settings.jsonc`의 `connections` (`internal/multiclient`가 하는 일) |
| 브라우저 | `localStorage`의 메타데이터 + 자기 origin |
| 매니저 | 구현하지 않음 — `Unimplemented` |

## 지금 무엇이 문제인가

웹 클라이언트는 연결을 하나만, 그것도 암묵적으로 가진다.

- `new Connection()`은 앱 부트스트랩에서 **한 번** 생성되고 (`ts/src/app/app.tsx:18,28`),
  `baseUrl`은 기본값 `location.origin` — 즉 브라우저가 접속한 주소 그 자체다
  (`ts/src/shared/api/connection.ts`).
- `WorkspaceContext`는 `c: Connection` **단수**다 (`ts/src/app/workspace-context.tsx`).
- 저장되는 것은 없다. 클래스 주석이 정책을 적어둔다: "No credentials or conversation
  cache are persisted in localStorage. Drafts use tab-local storage." 자격증명은
  게이트웨이의 HttpOnly 세션 쿠키뿐이다.

경계 자체는 잘 그어져 있다. `Connection` 하나가 transport·store·queries·inventory·
conversations·drafts·pastes·editors·attention을 **한 덩어리로** 들고 `dispose()`로
통째로 버려진다. 생성자가 `baseUrl`, 주입 가능한 `Transport`, 업로드 경로용
`transfer`를 받으므로 두 번째 연결이 필요해질 때 바꿀 자리도 이미 있다 — 샌드박스와
스토리북이 이미 그 형태로 `Connection`을 만든다 (`inProcess`).

없는 것은 **목록과 그것을 보여줄 자리**다. direct 접속(`npm run dev`의
`localhost:5173`을 포함해)이면 연결은 정확히 하나이고, 그 하나가 읽기 전용으로
보이는 것이 맞다. 지금은 하나조차 보이지 않는다.

## 왜 리소스로 정의하고 서버는 Unimplemented인가

payday 스택은 모든 서비스를 선언하고, 그 개념을 소유하지 않는 서버는 선언을 비워두지
않고 **거절을 반환한다**. 같은 파일에 선례가 있다 (`server/lifecycle/stack.go:108-117`):

```go
// Authentication/batch administration is intentionally not exposed through the
// local operator API. No dummy tenant or fabricated user is provisioned.
func (s Layer) Tenant() resource.TenantServiceServer { return resource.UnimplementedTenantServiceServer{} }
func (s Layer) Holder() resource.HolderServiceServer { return resource.UnimplementedHolderServiceServer{} }
func (s Layer) Outbox() resource.OutboxServiceServer { return resource.UnimplementedOutboxServiceServer{} }
func (s Layer) Audit()  resource.AuditServiceServer  { return resource.UnimplementedAuditServiceServer{} }
```

그래서 "매니저도 구현을 내놔야 한다"는 것은 비용이 아니라 이 스택의 정상 동작이다.
계약이 한 곳에 있으면 TUI와 브라우저가 같은 메시지를 쓰고, 나중에 허브가 생겨
연결을 **정말로 서버가 제공**하게 되면 클라이언트 코드를 바꾸지 않고 구현만 옮긴다.

### entity로 선언한다 — 선택의 여지가 없었다

처음에는 "CRUD 없이 서비스 RPC만" 선언하려 했다. 그럴 수 없다: pd는 서비스를
**entity에서** 생성하므로, entity가 아니면 `ConnectionService`도 `resource.Server`의
접근자도 생기지 않는다. 그래서 `(payday.entity)`로 선언하고, 매니저가 쓰지 않을 ent
테이블 하나와 `domain` 번호 하나를 비용으로 받아들인다. 대신 계약이 다른 리소스와
같은 자리에 있고, 클라이언트들은 생성된 클라이언트로 읽는다.

필드 번호는 5부터 시작한다. payday는 2와 3을 테넌트와 set 자리로 예약하며, 3을
가져가면 그렇다고 말해준다 — `field 3 is the set this row is in`.

그리고 생성된 서비스는 **ext 파일이 import를 공급해야** 자기 메시지를 볼 수 있다.
RPC를 하나도 더하지 않더라도 `proto/ext/cxz/connection_svc.ext.proto`가 필요한
이유다.

## 계약

`proto/cxz/connection.proto`와 `proto/ext/cxz/connection_svc.ext.proto`.

```proto
message Connection {
  // 표시 이름. settings.jsonc의 키, 또는 브라우저가 붙인 이름.
  string name = 1;
  // 어디에 붙는지. local://, ssh://, mtls://, tcp://, unix://, 또는 브라우저의 origin.
  string target = 2;
  // 이 클라이언트의 기본 연결.
  bool is_default = 3;
  // 실제로 무엇으로 붙어 있는지: local, ssh, mtls, tcp, origin.
  string transport = 4;
  // 지울 수도 바꿀 수도 없는 연결. 브라우저가 접속해 있는 origin이 그렇다.
  bool read_only = 5;
}
service ConnectionService {
  rpc List(ConnectionListRequest) returns (ConnectionListReply);
}
```

**자격증명은 들어가지 않는다.** 토큰, 토큰 파일 경로, 인증서 경로 모두 제외한다 —
`cxz connection ls`가 지키는 규칙과 같다. 목록은 "어디에 붙어 있는지"를 말하고,
"어떻게 증명하는지"는 말하지 않는다.

### `local://`이란

엔드포인트 스킴 중 하나이고 (`ssh://`, `mtls://`, `tcp://`, `local://`,
`unix:///path` — `internal/transport/endpoint.go:134`), **이 기계에 설치된 cxz**를
뜻한다. 특별한 이유가 있다: 설치된 매니저의 소켓은 Docker 안에 있고 호스트에
노출돼 있다고 가정할 수 없다. 그래서 `local://`은 주소로 붙지 않고 클라이언트 상태
디렉터리의 **설치 기술자**를 따라간다 (`cmd/cxz/remote.go:97`):

```go
if e.Scheme == "local" {
    if e.Address != "" { state = e.Address }  // local:///다른/state = 같은 기계의 다른 설치본
    return transport.Dial(state)
}
```

상태 디렉터리가 없으면 애초에 성립하지 않는다 (`endpoint.go:260`). 목록에서 중요한
이유는 이것이다: `connections`를 설정하면 평범한 `cxz`가 **모든** 연결의 프로젝트를
한 패널에 모으므로, `local://`을 함께 적어두지 않으면 자기 설치본이 그 목록에서
사라진다 (`docs/remote.md:34`).

브라우저에는 상태 디렉터리도 Docker 소켓도 없으므로 `local://`에 대응하는 것이
없다. 브라우저에서 "내가 서비스받고 있는 설치본"은 **자기 origin**이고, 그것이
목록의 읽기 전용 첫 줄이 된다. 같은 자리, 다른 이름이다.

## 단계

1. **계약** — 위 proto 두 파일, `go tool pd gen .`과 `go tool pd gen --ts .`
   (CI가 둘 다 검사한다).
2. **서버: Unimplemented** — `server/lifecycle/stack.go`에 `Connection()` 접근자와,
   매니저가 이 개념을 갖지 않는 이유를 적은 주석.
3. **브라우저 구현** — `ts/src/shared/api/connections.ts`가 `ConnectionService`를
   **브라우저 안에서** 구현한다. 네트워크 홉은 없다. 저장은 `localStorage`에
   **메타데이터만**: 이름, origin, 마지막 사용. 자격증명은 저장하지 않는다.
   현재 origin은 항상 첫 줄이고 `read_only: true`, `is_default: true`.
4. **UI 자리** — 설정의 `Connections` 토픽. 거대한 설정 폼에 끼우지 않고 자기 페이지로
   렌더한다: 연결은 설정이 아니라 읽는 것이라 폼이 아니라 표이고, 그 페이지에는
   `settings.json` 편집기도 없다. `WorkspaceContext`의 단수 `c: Connection`은 그대로
   두고 목록은 표시 전용이다. 여러 연결을 **동시에 집계**하는 것(지금 TUI가 하는 일)은
   `Connection` 인스턴스 N개가 필요해 범위가 다르다.
5. **TUI/CLI 구현** — `internal/multiclient`가 이미 가진 데이터를 같은 계약으로.
   당장 필요하지 않지만, 계약이 한 곳에 있다는 것을 증명한다.
6. **게이트웨이 origin** — 브라우저가 서로 다른 설치본에 직접 붙으려면 게이트웨이를
   고쳐야 한다. 지금은 단일 origin만 신뢰한다 (`internal/webui/server.go:141-146`:
   `r.Host`가 다르거나 `Origin` 헤더가 설정값과 다르면 403). 필요한 것:
   - 허용 origin 목록과 CORS 프리플라이트. Connect는 `Connect-Protocol-Version`,
     `Connect-Timeout-Ms` 같은 헤더를 쓰므로 `Allow-Headers`와 트레일러용
     `Expose-Headers`까지.
   - 자격증명 운반. 두 모양이 있다:
     - **cross-site 쿠키**: `SameSite=Strict`(`server.go:247`)를 `None; Secure`로
       바꾸고 `stream-fetch.ts:14`의 `credentials`를 `include`로. HttpOnly가 남으니
       XSS가 토큰을 읽지 못하지만, `Strict`가 공짜로 막아주던 CSRF 표면이 생긴다.
     - **연결별 Bearer 토큰**: `Authorization` 헤더로 보내고 탭 메모리에만 둔다.
       쿠키 정책을 건드리지 않고 CSRF 문제도 생기지 않는다. 대신 탭이 열려 있는
       동안 토큰이 JS 메모리에 있다.

     다중 origin이라면 **Bearer** 쪽을 권한다.
   - 에디터/터미널 WebSocket과 첨부 프록시도 같은 origin 검사를 지나므로 함께.

1~4가 한 PR이다. 5와 6은 각각 따로이며, 6은 보안 표면을 넓히는 결정이라 그 자체로
판단한다.

## 지켜야 할 것

- **자격증명은 영속하지 않는다.** `localStorage`에 들어가는 것은 이름과 origin뿐이다.
  `localStorage`와 OPFS는 XSS에 대해 동등하게 노출되므로 OPFS가 더 안전하지 않다 —
  차이는 크기와 스트리밍이고, 연결 목록은 몇 줄이라 OPFS는 과잉이다. 오프라인 대화
  캐시를 하기로 정하는 날 꺼내면 된다.
- **목록은 비밀을 말하지 않는다.** 토큰 파일 경로도 포함하지 않는다.
- **direct는 하나다.** 자기 origin 연결은 지울 수 없고, 그것이 유일한 연결일 수 있다.

## 어떻게 확인할 것인가

- 브라우저 구현의 단위 테스트: 빈 `localStorage`에서 목록이 자기 origin 한 줄이고
  `read_only`인지, 추가한 연결이 메타데이터만 담는지, 저장된 값에 토큰처럼 보이는
  것이 없는지.
- 스토리북/`test:browser`: 연결이 하나일 때 목록이 한 줄이고 삭제 수단이 없는지.
- 서버: `ConnectionService.List`가 `Unimplemented`를 반환하는지. 그것이 계약이다.
