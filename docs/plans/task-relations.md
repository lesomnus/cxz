# Task 사이의 관계 — 설계 계획

상태: **설계 결정 완료, 구현 전.** 결정 여섯 개:

| | 결정한 것 | 어디 |
| --- | --- | --- |
| 1 | 막힘은 파생 값이다. `status`는 서버가 건드리지 않고, 집행은 `Claim`의 거부 한 곳 | §6 |
| 2 | 링크 추가·삭제에 task 소유권을 요구하지 않는다 | §5 |
| 3 | 프로젝트 간 링크는 범위 밖. 거부하고, 왜인지와 무엇이 먼저인지 적는다 | §2 |
| 4 | 순환은 거부한다 | §5 |
| 5 | `kind` enum은 지금 정의하고 `BLOCKS`만 구현한다 | §4, §9 |
| 6 | 끝난 blocker의 링크는 남긴다. 상한은 열린 것만 센다 (프로젝트당 500, task당 32) | §5 |

지금은 "이 task는 저 task에 막혀 있다"를 적을 자리가 없다. 이 문서는 없는 것을
측정하고(§1, §2), 다른 도구 8개가 같은 문제를 어떻게 푸는지 보고(§3), 모양과
집행 지점을 정하고(§4, §5, §6), 표면과 단계를 나눈다(§7, §8). §9는 결정의 근거와
남은 것이다.

## 1. 지금 없는 것

`Task`(`proto/cxz/task.proto`)의 필드 전부: `id` `alias` `name` `desc` `labels`
`project`(edge) `status` `priority` `assignee` `assignee_run` `client_id`
`source_session` `source_run` `source_seq` `date_created` `date_updated`
`date_erased`. 다른 Task를 가리키는 필드도, edge도 없다. 오버레이
(`proto/ext/cxz/task_svc.ext.proto`)가 더하는 것은 `Claim`/`Release` 두 RPC와
라우팅용 `ProjectRef project = 1000` 세 개, `TaskActor`뿐이다.

가장 가까운 두 개, 그리고 왜 관계가 아닌가:

- **`map<string, string> labels = 7`** — `blocked-by: cxz-42`를 넣을 수는 있다.
  MCP의 `task_add`/`task_patch`도 `labels`를 그대로 받는다. 그런데 아무것도 그
  값을 해소하지 않고, 검증하지 않고, 그걸로 거를 수도 없다 — `list`의 필터는
  `ref`/`project`/`assignee` 셋뿐이다(`task.proto:39`). `alias`는 변경 가능하므로
  이름을 바꾸면 라벨은 조용히 끊어진다.
- **`TASK_STATUS_WAITING`** — "막혀 있다"는 말할 수 있지만 **무엇에** 막혔는지는
  말할 수 없다.

비교: 세션은 포크의 유래를 `Session.parents`로 적는다. task에는 그 상응물이 없다.

## 2. 저장 위치가 미리 정하는 것

두 가지가 설계 공간을 좁힌다. 둘 다 코드에서 읽은 사실이다.

**(a) task 행은 프로젝트 런타임의 `resources.db`에 있고, 매니저는 프록시다.**

매니저와 프로젝트 런타임은 **같은 바이너리에 같은 구현**이다. 서비스가 두 개 있는 것이
아니다. 역할은 환경변수 하나로 갈리고(`manager := os.Getenv("CXZ_OWNER") != ""`,
`internal/server/server.go:114`), 갈리는 지점은 한 군데뿐이다:

```go
func (s *Server) TaskConnection(ctx context.Context, project string) (*grpc.ClientConn, error) {
	if s.manager == nil {
		return nil, nil                                // 내가 그 런타임이다 -- 내 resources.db로 답한다
	}
	return s.manager.ClientForProject(ctx, project)    // 나는 매니저다 -- 그 프로젝트로 넘긴다
}
```
(`internal/server/tasks.go`. `TaskServer.target`이 이 값을 보고 자기 저장소와
프록시를 고른다 — `server/lifecycle/task.go:59`.)

그러니 "매니저에 구현이 없다"가 아니라 **"매니저에 그 데이터가 없고, 있는 척하기를
거부한다"**다. 주석이 그 말을 한다 — "Never fall back to a manager-side copy when the
authoritative project is stopped/unreachable"(`task.go:40`). 결과 셋:

- 모든 task 쓰기는 **한 프로세스**(그 프로젝트의 런타임)를 지난다. §5의 순환 검사가
  경쟁 조건 없이 성립하는 근거가 이것이다.
- **멈춘 프로젝트의 task는 읽히지 않는다.** `ClientForProject`가 실패하면 호출 전체가
  실패한다. 의도된 동작이다.
- 매니저는 팬아웃 수단을 이미 갖고 있다 — 프로젝트마다 `ClientForProject`를 부르면
  된다. 프로젝트 간 읽기가 막힌 것은 연결이 아니라 저장과 일관성이다.

**(b) task는 지워지지 않는다.** `taskapi.Service.Erase`는 거부한다 —
"cancel tasks instead of deleting shared work history"(`internal/taskapi/service.go:362`).
그래서 링크가 사라진 행을 가리키는 경우를 다룰 필요가 없다. 대상은 항상 거기
있고, `DONE`이거나 `CANCELED`일 뿐이다.

### 프로젝트 간 링크 (결정 3: 범위 밖)

양쪽 행이 다른 데이터베이스 파일에 있으므로 하나의 조인 행으로 적을 수 없다.
`Add`는 두 task가 같은 프로젝트에 있는지 확인하고, 아니면 거부한다 — 에러 메시지에
"링크는 같은 프로젝트 안에서만"을, `task_link.proto`의 주석에 왜인지(위 (a))와 풀려면
무엇이 먼저인지(매니저 측 프로젝트 간 읽기)를 적는다.

나중에 지원하게 될 때를 위해, 이 설계가 **잠그지 않는 것**:

- **와이어는 이미 그것을 말할 수 있다.** `TaskLinkAddRequest`의 `source`/`target`은
  `TaskRef`이고, 오버레이가 거기에 `ProjectRef project = 1000`을 붙여 둔다
  (`TaskLinkFilter`도 같은 `TaskRef`를 쓴다). 지원을 켤 때 요청 메시지에 필드를 더할
  일이 없으므로 클라이언트 API도 TS 생성물도 깨지지 않는다.
- **저장 형태를 바꿀 때의 마이그레이션이 기계적이다.** edge를 버리고 스칼라 id로 가야
  한다면, `target_id` 열이 이미 그 UUID를 들고 있다. 새로 채울 프로젝트 열은 기존 행이
  전부 같은 프로젝트 안의 링크이므로 그 task의 `project`를 복사하면 된다.
- **결정적 `client_id`도 안 걸린다.** task id는 pdid UUID라 전역 유일하므로
  `<source>:<kind>:<target>` 키가 프로젝트를 넘어서도 유일하다.

반대로 이 설계가 **전제하는 것**, 즉 그때 깨야 할 것:

- `source`/`target`이 edge라서 같은 DB의 외래 키다. 스칼라로 바꾸면 참조 정합성,
  `TaskLinkSelect`의 `TaskSelect target`(한 번의 호출로 상대 행을 같이 읽는 것),
  ref 기반 필터를 잃는다. 지금 실제로 쓰는 것들이다.
- §5의 순환 검사가 "그래프 전체가 한 DB, 한 프로세스"를 쓴다.

그리고 진짜 벽은 그 둘도 아니다: **링크 행이 어느 DB에 사는가.** p1의 A와 p2의 B를
잇는 링크를 p1에 두면 p2의 watch 스트림에는 보이지 않고, p2가 "나는 무엇에 막혔나"에
답하려면 p1을 읽어야 한다. 게다가 (a)에 따라 상대 프로젝트가 멈춰 있으면 §6의 파생
`blocked`를 계산할 수 없다. 즉 프로젝트 간 링크는 이 설계가 막아서 어려운 것이 아니라,
그보다 큰 기능(매니저가 여러 런타임에 팬아웃해서 읽고 일관성을 정의하는 것) 뒤에 줄 서
있다. task에 대한 프로젝트 간 읽기는 지금 전부 의도적으로 막혀 있다 —
`TaskServer.target`은 project를 필수로 받고("project is required"),
`NormalizeFilters`는 스코프를 벗어난 필터를 거부한다
("filter is outside project scope", `internal/taskapi/service.go:324`).

지금 할 수 있는 유일한 미래 대비는 edge를 포기하고 처음부터 스칼라 id로 저장하는 것인데,
대가는 오늘 확실하고 이득은 가정적이다. edge로 간다.

## 3. 다른 솔루션은 어떻게 하나

여덟 개를 봤다. 보려던 것은 셋이다: 관계의 종류, **순환을 막는가**, 그리고 **막힌
상태가 task의 상태를 바꾸는가**.

| 도구 | 관계 종류 | 역방향 | 순환 | 막힘의 효과 |
| --- | --- | --- | --- | --- |
| Jira | blocks / is blocked by, clones, duplicates, relates to (추가 가능) | 양쪽에 자동 표시 | **막지 않는다** (비공식 출처) | 상태 변화 없음. Advanced Roadmaps는 finish-to-start로 일정만 밀어낸다 |
| GitHub Issues | blocked by / blocking (sub-issue는 별개 기능) | 자동 | 문서에 없음 | 보드와 목록에 "Blocked" 아이콘. 닫기를 막는다는 서술 없음 |
| GitLab | relates to (Free), blocks / is blocked by (Premium) | 자동 | 문서에 없음 | 닫을 때 **경고**, 무시 가능. 아이콘 |
| Linear | blocked / blocking / related / duplicate | 자동 역방향 | 문서에 없음 | blocker가 끝나면 초록으로 바뀌어 Related로 내려간다. 상태는 안 바꾼다 |
| Redmine | relates, duplicates, blocks, precedes/follows, copied to | 상호 관계로 저장 | 문서에 없음 (precedes는 날짜 규칙) | blocks는 **닫기를 막는다**. 상태는 안 바꾼다 |
| Asana | dependencies (blocked by / blocking) | 자동 | **2개 고리는 거부, 3개 고리는 통과** — 직원이 "product limitation"이라 답했다 | 해제되면 Inbox 알림. 상태는 안 바꾼다 |
| Azure DevOps | Related, Parent-Child, Predecessor-Successor, Duplicate | 자동 | **거부한다** — "An error appears when you attempt to create links that define circular relationships" | 상태 변화 없음 |
| Taskwarrior | `depends:` 한 방향만 | 저장하지 않고 파생: 가상 태그 `BLOCKED`/`BLOCKING`/`UNBLOCKED` | **거부한다** — `src/Task.cpp`가 `dependencyIsCircular`를 호출하고 "Circular dependency detected and disallowed."를 던진다. 테스트 `test_circular_1/2` | 상태 변화 없음. `ready` 리포트에서 빠지고 `blocked` 리포트에 들어간다 |

읽어낸 것 셋:

1. **여덟 중 어느 것도 관계 때문에 상태를 자동으로 바꾸지 않는다.** 막혀 있다는 것은
   전부 **파생 신호**다 — 아이콘(GitHub, GitLab), 가상 태그와 리포트(Taskwarrior),
   사이드바 구획(Linear). 상태는 사람이 둔 값으로 남는다.
2. **역방향을 두 행으로 저장하는 곳은 없다.** 한 방향을 적고 반대쪽은 조회로 만든다.
3. **순환은 도구의 성격이 갈라놓는다.** 기계가 소비하는 의존성(Azure DevOps의
   predecessor/successor, Taskwarrior의 `depends`)은 거부한다. 사람이 읽는
   트래커(Jira, Asana)는 방치하고, 두 출처 모두 그 결과를 문제로 적는다 — Asana는
   자기 제품의 한계라고 부르고, Jira 쪽 글은 "에러는 안 나지만 보드가 멈춘다"고
   쓴다. cxz의 소비자는 에이전트다. 거부하는 쪽이 맞고, 그게 이미 내린 결정이다.

출처: [Redmine Issues](https://www.redmine.org/projects/redmine/wiki/RedmineIssues) ·
[Linear issue relations](https://linear.app/docs/issue-relations) ·
[GitHub creating issue dependencies](https://docs.github.com/en/enterprise-cloud@latest/issues/tracking-your-work-with-issues/using-issues/creating-issue-dependencies) ·
[GitHub CLI 2.94 changelog](https://github.blog/changelog/2026-06-10-manage-sub-issues-types-and-dependencies-from-github-cli/) ·
[GitLab linked issues](https://docs.gitlab.com/user/project/issues/related_issues/) ·
[Azure DevOps link type reference](https://learn.microsoft.com/en-us/azure/devops/boards/queries/link-type-reference) ·
[Asana forum: 순환 의존성](https://forum.asana.com/t/why-can-i-create-a-circular-dependency-of-three-tasks/74295) ·
[Taskwarrior task(1)](https://man.archlinux.org/man/task.1.en) ·
[Jira linked issues (비공식)](https://ones.com/blog/jira-linked-issues-types-blocks-clones-more-explained/) ·
[Jira Portfolio dependencies](https://confluence.atlassian.com/jiraportfoliocloud/managing-portfolio-dependencies-828785188.html)

## 4. 모양: `TaskLink` 조인 엔티티

`Task`에 self edge를 달지 않는 이유는 edge가 하나만 가리키기 때문이다. 하나의 task가
여러 task에 막힐 수 있고 관계에 종류가 붙으므로 다대다이고, 이 저장소에서 그것은
별도 엔티티다.

아래는 **실제로 생성해 본** 정의다. `pd gen`과 `go build ./...`가 통과했고, 측정한
것은 그 아래에 적는다. (커밋에는 포함하지 않았다 — 이 PR은 문서다.)

```proto
message TaskLink {
  bytes id = 1 [(orm.field) = {type: TYPE_UUID, key: true, default: ""}];
  google.protobuf.Timestamp date_updated = 13 [(orm.field) = {version: {}}];
  google.protobuf.Timestamp date_erased = 14 [(orm.field) = {erased: {}}];
  google.protobuf.Timestamp date_created = 15 [(orm.field) = {immutable: true, default: ""}];
  Task source = 8 [(orm.edge) = {immutable: true}];
  Task target = 9 [(orm.edge) = {immutable: true}];
  TaskLinkKind kind = 10 [(orm.field) = {immutable: true}];
  string client_id = 16 [(orm.field) = {unique: true, immutable: true}];
  option (orm.message) = {
    rpc: {crud: true}
    indexes: [{name: "page", refs: [{name: "date_created", number: 15}, {name: "id", number: 1}]}]
  };
  option (payday.entity) = {
    domain: 13
    global: {}
    list: {order: [{field: {name: "date_created"}}, {field: {name: "id"}}], by: [{name: "ref"}, {name: "source"}, {name: "target"}], size: 100, max: 500}
    watch: {}
  };
}

enum TaskLinkKind {
  TASK_LINK_KIND_UNSPECIFIED = 0;
  TASK_LINK_KIND_BLOCKS = 1;
  TASK_LINK_KIND_RELATES = 2;
}
```

방향은 **`source`가 `target`을 막는다**로 읽는다. 한 행만 저장하고, "내가 막힌 것"은
`target`으로 거는 List, "내가 막는 것"은 `source`로 거는 List다 — §3의 교훈 2.

측정한 것:

- **같은 타입을 가리키는 edge 두 개는 받아들여진다.** 생성된 ent 스키마
  (`internal/ent/schema/task_link.go`)는 `edge.To("source", Task.Type)`와
  `edge.To("target", Task.Type)`를 각각 `Unique().Required().Immutable()`로 내고,
  FK 열은 `source_id`/`target_id`다.
- **`kind`를 `list: by:`에 넣으면 거부된다.** 정확한 에러: *"cxz.TaskLink: list:
  by: \"kind\" is TYPE_ENUM, and a filter compares for equality -- which is not
  something this generator writes for that."* 그래서 위 정의의 `by:`에는 `kind`가
  없고, 종류별 선별은 레이어가 한다. 링크는 task당 수십 개 규모이므로
  `by: target` 한 번 읽고 메모리에서 걸러도 비용이 없다.
- **생성되는 계약**(`proto/cxz/task_link_svc.g.proto`): `Add` `Get` `Patch` `Apply`
  `Erase` `List` `Watch`. `TaskLinkFilter{ref, source, target}`,
  `TaskLinkRef{oneof id | client_id}`, `TaskLinkAddRequest`는 `TaskRef source`와
  `TaskRef target`을 받는다 — 그 `TaskRef`는 오버레이가 `ProjectRef project = 1000`을
  붙여 둔 그 메시지다.
- **`Patch`에 쓸 수 있는 필드가 하나도 없다.** 전부 immutable이라 생성된
  `TaskLinkPatchRequest`에는 버전 필드만 남는다. 그러니 `Patch`/`Apply`는 레이어가
  거부한다. 반대로 **`Erase`는 허용한다** — 링크를 끊는 것이 그것이고, task 자체를
  지우는 것(§2(b))과는 다른 판단이다.
- **`client_id`에 부분 유니크 인덱스가 걸린다**(`date_erased IS NULL`인 행에 대해).
  그래서 중복 링크는 결정적 `client_id`로 막는다: `<source id>:<kind>:<target id>`.
  같은 링크를 두 번 요청하면 재시도가 되고, 레이어의 읽기-후-쓰기에 의존하지 않는다.
- `resource.Server` 인터페이스에 `TaskLink()`가 추가되고, `pdid` 도메인 13이
  `"task-link"`로 등록되며, 손으로 쓴 코드 없이 `go build ./...`가 통과한다.
  도메인 13은 비어 있다 (쓰이는 것: payday 1–4, cxz 7–12).
- **오버레이가 해야 할 일**: `TaskLinkRef`, `TaskLinkListRequest`,
  `TaskLinkWatchRequest`에 `ProjectRef project = 1000`을 더한다. §2(a) 때문에
  필수다 — 프로젝트를 모르면 매니저가 어느 런타임으로 보낼지 알 수 없고, 아무것도
  안 하면 매니저 자신의 `resources.db`를 답하게 된다. `Task`가 그렇게 하는 것과 같은
  이유, 같은 필드 번호다.

## 5. 레이어의 규칙 — 순환 거부, 소유권, 상한

**어디서**: 생성된 sink가 아니라 `TaskLinkService.Add`를 가로채는 레이어. `Task`가
`internal/taskapi`에 규칙을 두고 `server/lifecycle/task.go`가 라우팅하는 것과 같은
배치로, `internal/taskapi`에 링크 규칙을 더하고 `server/lifecycle`에
`TaskLinkServer`를 둔다.

**규칙 다섯**, 전부 `Add`에서:

1. `source == target`이면 거부한다.
2. 두 행이 같은 프로젝트에 있어야 한다 (§2의 결정 3).
3. `kind == BLOCKS`일 때: 새 간선 `A → B`를 넣기 전에 `B`에서 출발해 `BLOCKS` 간선을
   따라가 `A`에 닿는지 본다. 닿으면 거부한다. `InvalidArgument`로, 어떤 고리가
   닫히는지를 메시지에 적는다.
4. `kind == RELATES`는 지금 거부한다 (`InvalidArgument`). enum 값은 정의해 두고 구현은
   5단계에서 UI와 함께 켠다 — §9.
5. 상한을 넘으면 거부한다 (아래).

**순환 검사는 쿼리 한 번으로 끝낸다.** 노드마다 쿼리를 날리는 대신, 그 프로젝트의
`BLOCKS` 링크를 한 번에 읽어 메모리에서 탐색한다. 생성된 List의 페이지 상한이
500이므로(§4의 `size: 100, max: 500`) 프로젝트의 열린 링크가 500개 아래면 그래프 전체가
한 번의 쿼리로 손에 들어온다. 그래서 "방문 노드 상한" 같은 별도 개념이 필요 없다.

**상한 둘** (결정 6, 둘 다 `internal/taskapi`의 상수라 숫자는 한 줄로 고친다):

- 프로젝트당 **열린** `BLOCKS` 링크 500개 — 한 페이지. 끝난 blocker의 링크는 세지
  않으므로, 기록을 남기면서도(§9) 상한은 실제로 막혀 있는 것만 센다.
- task당 32개. 선례는 라벨 상한 32다 (`internal/taskapi/service.go:377`).

**소유권은 요구하지 않는다** (결정 2). 조건은 두 행이 같은 프로젝트에 있는 것과 위의
규칙들뿐이다. `Task`의 기존 규칙과 일관된다 — 소유권은 **실행 상태 전이에만** 필요하고
`name`·`desc`·`priority`·`labels`·`alias`는 남의 task라도 고칠 수 있다
(`internal/taskapi/service.go:214`–`231`, 취소는 예외라는 주석이 `:222`에 있다).
그리고 막혔다는 사실은 보통 소유자가 아닌 쪽이 먼저 발견한다. 소유권 요구는 §6의
`Claim` 거부 한 곳으로 충분하다.

**`Erase`도 같다** — 누구나 링크를 끊을 수 있다. 링크를 끊는 것은 task를 지우는 것과
다르다(§2(b)).

**경쟁 조건은 없다.** §2(a)에서 모든 task 쓰기가 그 프로젝트 런타임의 한 프로세스를
지나므로, 그 안의 프로젝트별 뮤텍스가 "읽어서 검사하고 쓴다"를 원자적으로 만든다.
두 세션이 동시에 서로 반대 간선을 넣어 양쪽 다 통과하는 경우가 이것 없이는 생긴다.

**`RELATES`가 켜질 때**(5단계): 무방향이므로 순환이라는 개념이 성립하지 않는다. 한 행만
저장하고 양쪽에서 조회하며, 막을 것은 중복뿐이다 — 그건 §4의 결정적 `client_id`가
한다. 다만 `A relates B`와 `B relates A`는 서로 다른 `client_id`가 되므로, 레이어가
`RELATES`를 저장할 때 두 id를 정렬해 넣는다.

## 6. 상태 정책 (결정 1: A)

§3의 교훈 1이 그대로 답이다. 자동으로 상태를 바꾸는 시스템은 하나도 없었다. 선택지는
셋이었다 — A(상태 불변 + `Claim` 거부), B(자동 `WAITING`), C(표시만 하고 집행 없음).
**A로 간다.**

**A — 막힘은 파생 값이고, 집행은 `Claim` 한 곳에서만 한다.**

- `blocked(task)` = `kind == BLOCKS`이고 `target == task`인 링크 중, `source`의 상태가
  `DONE`도 `CANCELED`도 아닌 것이 하나 이상 있다. 저장하지 않고 읽을 때 계산한다.
- **`TASK_STATUS_WAITING`의 뜻은 건드리지 않는다.** 사람이나 에이전트가 직접 두는
  상태로 남는다. (지금 `Claim`은 자기 run이 소유한 `IN_PROGRESS`/`WAITING` 작업의
  재claim을 허용한다 — `internal/taskapi/service.go:260`. 그 동작도 그대로다.)
- **유일한 집행은 `Claim`의 거부**다. 막힌 task를 claim하면 `FailedPrecondition`으로
  무엇에 막혔는지(blocker의 alias) 말한다. 자리는 이미 있다: `Claim`은 이미 "already
  owned or not Todo"를 같은 코드로 거부한다(`service.go:264`).
  왜 여기인가 — Taskwarrior가 `ready`에서 막힌 일을 빼는 것과 같은 자리이고, cxz에서
  일을 집어 드는 주체는 에이전트다. 아이콘은 에이전트에게 아무 의미가 없고, 막힌 일을
  시작해 버린 에이전트를 되돌리는 비용은 거부의 비용보다 크다. Redmine은 *닫기*를
  막지만, 우리에게 비싼 것은 시작이다.
- **그 거부는 사람에게 한 번도 닿지 않는다.** 웹 UI는 `claim`을 호출하지 않는다 —
  `ts/src/features/tasks/`가 쓰는 것은 `add`/`get`/`patch`/`release`뿐이다. `Claim`은
  MCP 경로, 즉 에이전트 전용이다. 사람이 웹에서 막힌 task를 편집하거나 상태를 바꾸는
  것은 아무 제약 없이 그대로 된다.
- **`DONE`/`CANCELED`로 가는 전이는 막지 않는다.** 막힌 일을 포기하거나 취소하는 길을
  남겨야 한다. GitHub·GitLab도 닫기를 막지 않는다.
- 목록은 막힘을 **보여주기만** 한다. 파생 값이므로 서버 측 필터가 되지 않는다
  (§4: `kind`로 못 걸고, 애초에 다른 엔티티다). 호출자가 `TaskLinkService.List`를 한 번
  더 하거나, MCP의 `task_list`가 그 조합을 대신한다 — §7.

**기각한 B — 링크가 생기면 상태를 자동으로 `WAITING`으로 바꾼다.** 비용 둘. 서버가
사람이 둔 상태를 덮어쓴다. 그리고 blocker가 끝났을 때 무엇으로 되돌릴지 알 수 없다 —
`TODO`였는지 `IN_PROGRESS`였는지 기록이 없고, 기록하려면 "이전 상태" 열을 새로 만들어야
한다(proto 변경). 벤치마크한 여덟 중 이렇게 하는 것은 없다.

**기각한 C — 표시만 하고 집행하지 않는다.** 벤치마크한 여덟 중 여섯이 이쪽이다. 하지만
그 여섯의 독자는 사람이고, 여기서 일을 집어 드는 것은 에이전트다. C는 막힌 일을 그냥
시작하게 둔다. A는 C 위에 거부 한 줄을 얹은 것이므로, A가 과하다고 판명되면 그 한 줄을
지워 C로 돌아갈 수 있다.

**A를 고르면 따라오는 제약**: §8의 3단계(거부)와 4단계(MCP 도구·뷰)는 **한 PR로 같이
가야 한다.** 목록에 막힘이 보이지 않는 상태로 거부만 켜면 에이전트가 예측할 수 없는
거부를 받고 재시도만 한다. 거부 메시지가 blocker를 적어주고 `task_list`가 막힘을
표시해야 "그럼 blocker를 먼저 하자"로 갈 수 있다.

## 7. 표면

- **MCP**(`internal/taskmcp/server.go`, 지금 도구 6개): `task_link`와 `task_unlink`를
  더한다. `task_get`의 뷰에 `blocked_by`/`blocks`를 더하고(`related`는 4단계),
  `task_list`에 막힘 표시와 "막히지 않은 것만" 옵션을 둔다. `Instructions`에 한 줄:
  막힌 일은 claim 되지 않으니 그걸 풀거나 다른 일을 집어라.
- **웹**(`ts/src/features/tasks/`): 카탈로그가 `TaskService.Watch` 하나로 사는 구조라
  (`task-catalog.ts`) 링크용 watch 스트림이 하나 더 붙고 보드에서 합쳐진다. 보드에
  배지, `task-dialog.tsx`에 링크 구획.
- **CLI**: 없다. task에는 CLI 표면이 없다 (`cmd/cxz`에 `task_mcp.go`뿐).
- **스텁 둘도 같이 가야 한다**: `internal/taskapi/memory.go`(샌드박스가 쓴다,
  `internal/websandbox/server.go:67`)와 `ts/src/storybook/task-fixture.ts`.

## 8. 단계

1. `proto/cxz/task_link.proto` + 오버레이(§4 마지막 항목) + `pd gen`. 손으로 쓸 코드
   없이 CRUD가 산다.
2. `internal/taskapi`에 링크 규칙(§5) + `server/lifecycle/task_link.go`의 라우팅 +
   `Patch`/`Apply` 거부. 테스트: 자기 링크, 2-고리, 3-고리, 프로젝트 교차, 중복 재시도,
   `RELATES` 거부, 상한.
3. 파생 `blocked` + `Claim` 거부(§6) + MCP 도구와 뷰(§7). **한 PR이다** — §6의 마지막
   항목이 이유다.
4. 웹과 스텁 둘(`internal/taskapi/memory.go`, `ts/src/storybook/task-fixture.ts`).
   `RELATES`를 여기서 켠다.

1–3이 핵이고 3 없이는 쓸 수 없다. 4는 뒤로 미뤄도 에이전트는 이미 쓸 수 있다.

## 9. 결정의 나머지와 범위 밖

- **`RELATES`는 enum 값만 정의하고 구현은 4단계로 미룬다** (결정 5). 서버 쪽 규칙은
  거의 없지만 가치가 거의 전부 UI에 있고, UI는 4단계다. 1–3단계에서는 `Add`가
  `InvalidArgument`로 거부한다. 에이전트에게도 "관련 있음"은 유용하지만("저쪽 설명을
  읽어라"), 지금 필요한 것은 막힘이고 둘을 같이 넣으면 §5의 규칙 테스트가 두 배가 된다.
- **끝난 blocker의 링크는 남긴다** (결정 6). Linear는 해제된 blocker를 Related 구획으로
  내려 보관한다. 자동으로 지우면 왜 막혀 있었는지가 사라진다. 링크 수가 단조 증가하는
  문제는 §5의 상한을 **열린** 링크만 세게 해서 해결했다.
- **`DUPLICATES`와 부모-자식은 범위 밖.** sub-task는 관계가 아니라 별개 기능이고,
  GitHub과 Linear도 sub-issue를 관계와 분리해 둔다. 필요해지면 그때 따로 센다.
- **아직 결정하지 않은 것은 없다.** 다시 열릴 자리는 둘이다: 프로젝트 간 링크가 실제로
  필요해지는 날(§2 — 그때는 이 설계보다 큰 기능이 먼저다), 그리고 `Claim` 거부가 과하다고
  판명되는 날(§6 — 한 줄 지우면 C다).
