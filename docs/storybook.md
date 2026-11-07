# Storybook

앱의 실제 React 컴포넌트와 CSS를 서버 연결 없이 확인합니다. 기본 언어는 영어입니다.
cxz Storybook은 제품 화면과 앱 연동 예제만 등록합니다. Nox 기본 컴포넌트는
별도 Nox Storybook에서 확인하며, 같은 스토리를 두 카탈로그에 등록하지 않습니다.

```sh
cd ts
npm ci
npm run storybook
```

브라우저에서 <http://localhost:6006>을 엽니다. 컨테이너나 원격 개발 환경에서는
6006 포트를 로컬로 포워딩합니다. `npm run dev`는 Vite와 이 Storybook을 함께
실행하며, Ctrl+C로 둘 다 종료합니다. Vite만 실행하려면 `npm run dev:app`을 사용합니다.

- `Components/AgentBrand`: cxz가 지원하는 에이전트의 로고와 이름.
- `Components/FontFamilyControl`: 로컬 글꼴과 Google Fonts 선택, 적용 후 저장되는 메타데이터. Google Fonts는 Apply font를 누를 때만 다운로드합니다.
- `Components/ConfirmationDialog`: 배경 조작과 포커스를 제한하는 확인창. 취소, 실행 대기, 실패 후 재시도, Purge 상태를 확인합니다.
- `Components/EditableValue`: 값 영역 그대로 편집하는 입력과 취소·확인 오버레이. Alias/Title, 저장 대기, 실패 후 수정·재시도를 확인합니다.
- `Components/SessionDetails`: 고정폭 key·value와 Title·Alias 편집. 편집 가능한 행의 hover 배경 안에서 값 영역은 더 밝은 둥근 표면을 유지하며, 외곽과 내부 모서리는 같은 중심을 갖습니다. Title은 `Session.name`이고 alias와 별개이며, 둘 다 `Session.Patch`로 저장합니다. 이름 변경은 수동 제목으로 기록하고 진행 중인 자동 제목 생성을 취소하며, 보조 AI를 실행하지 않습니다.
- `Conversation/TaskDetails`: 작업의 Input/Output/Result를 cxz `DetailTabs`와 Monaco로 표시하는 floating card 연동 예제. 기본 Card와 SourceEditor 자체 예제는 Nox에 있습니다.
- `Sessions/Panel`: 실제 세션 카드, 선택 하이라이트, 상태 indicator, 프로젝트 접기, 긴 목록의 스크롤. `WorkingPhases`는 여러 working 세션의 시작 위상과 오로라 색상 순서가 서로 다른 예제입니다. 세션 ID와 run ID로 seed를 만들므로 새로고침하거나 프로젝트를 접었다 펼쳐도 같은 배치를 재현하며, 이름 변경이나 일반 상태 갱신은 배치를 바꾸지 않습니다.
- `Pages/Projects`: Account와 같은 공용 Sessions 테이블을 사용합니다. 이름·alias를 묶은 세션/계정 이동 버튼, 상태 아이콘, 행 선택과 개별·일괄 작업, 세션 추가를 확인합니다.
- `Pages/Search`: 세션 검색과 같은 검색바, 프로젝트·Nox 날짜 필터, 빈 결과·오류·검색 중단·추가 결과 로딩 예제를 제공합니다.
- `Pages/Memory`: 기본 예제는 테두리 없는 세로형 메모리 카드 그리드입니다. 상단 왼쪽 에이전트 로고·세션 이름과 오른쪽 경과시간, 대화 본문 크기의 제목, 하단 왼쪽 리비전을 확인합니다. `CardGrid`는 긴 제목, 두 에이전트, 소유자 없는 문서와 종료된 세션을 포함하며 좁은 화면에서는 열 수가 줄어듭니다. 프로젝트 옆 세션 드롭다운으로 소유 세션을 필터하고 전체로 되돌릴 수 있으며 프로젝트 변경 시 초기화됩니다. `Document`는 목록 사이드바 없이 문서 경로로 직접 진입합니다. `MigratedDocuments`는 같은 파일명·여러 소유 세션·이관 라벨이 섞인 목록입니다. 별도 편집 모드, 읽기 전용 원문, 버전 복원, 복사, 삭제, 저장 충돌과 빈 프로젝트·로딩·오류 예제도 제공합니다.
- `Resources/Tasks`: 프로젝트 선택과 상태별 작업 보드. `ProjectBoard`, `ManyTasks`, `EmptyProject`, `OlderRuntime`에서 등록·수정, 담당 세션 이동, 정지/이전 run 경고와 Release 확인을 확인합니다. `Simulate agent update`는 열린 편집 내용을 보존하면서 원격 갱신·alias 변경과 저장 충돌을 재현합니다. 실제 Connect 클라이언트와 메모리 서비스 fixture를 사용하며 서버의 Task는 변경하지 않습니다.
- `Conversation/Composer`: Markdown, 코드블록, 붙여넣기 chip, 명령어 제안, 전송 대기, 작업 중 glow, 모델·effort와 usage 표시.
- `Workspace/Errors`: `Recovery`에서는 실제 사이드바의 에러 목록·회복·Dismiss·기록을 확인하고, 다이얼로그 상단 이동 버튼으로 독립 페이지를 엽니다. `Page`는 리소스 툴바와 테이블로 구성된 독립 페이지의 직접 진입 예제이며, `Overview`는 여러 에러와 History 항목을 넣어 행별 상세 펼치기와 좁은 화면 배치를 확인합니다. 마지막 발생 후 1시간 동안 재발하지 않은 항목은 해결됨과 구분되는 Stale 상태로 History에 이동하며, 두 화면에서 같은 상태를 공유합니다.
- `Workspace/Errors/Rows`: 등록된 작업별 행 표시 40개를 `All`에서 확인합니다. `RPC`, `HTTP`, `Connection`, `Runtime`, `Client`는 종류별 예제이고, `Dialog`는 메시지와 종류별 진단값·경과시간·횟수를 두 줄로 배치하고 Dismiss만 표시합니다. RPC Target은 종류명 없이 ID 앞 7자리 또는 이름·별칭을 표시하고, 경과시간은 `5m ago`처럼 축약합니다. Dismiss는 행 hover·키보드 포커스에서 높이를 채우는 그라디언트 오버레이로 표시하며 터치 환경에서도 조작할 수 있습니다. RPC 이름·상태 코드, HTTP 메서드·경로·응답 코드, 연결 종료 코드, 컨테이너 상태·실패 단계, 예외 종류·소스 위치를 해당 종류의 행에서 바로 확인합니다. `RPC dialog`, `HTTP dialog`, `Connection dialog`, `Runtime dialog`, `Client dialog`는 종류별 미리보기입니다. 바깥 스크롤바를 사용하며 푸터와 열 제목은 없습니다. `RPC details`는 독립 페이지 행의 상세를 펼쳐 추가 메타데이터를 보여줍니다. `cxz.SessionService/Transcript` 같은 전체 호출명은 행에도 그대로 표시됩니다.
- `Accounts/Cache`: 계정 정보·인증 상태는 5분, usage·잔디밭·남은 사용량은 1분 캐시합니다. 캐시 값은 메모리에서 동기적으로 읽어 애니메이션 없이 즉시 표시합니다. Account 정보·인증 상태·날짜도 정적으로 표시합니다. 화면을 닫고 다시 열 때의 RPC 횟수와 Erase 오른쪽 새로고침 버튼의 강제 갱신을 확인합니다. `SlowRefresh`는 로딩 피드백과 중복 클릭 차단을 확인하는 예제입니다. 프로젝트·계정 모두 새로고침 중 기존 잔디밭과 선택 날짜를 유지하고 새 응답이 도착하면 갱신합니다. `Pages/Projects/UsageRefresh`에서는 Change usage on server를 누른 뒤 Refresh하여 최근 통계와 잔디밭 응답 사이의 대기 구간을 확인할 수 있습니다. 다른 프로젝트로 전환할 때에는 이전 프로젝트의 잔디밭을 표시하지 않습니다. 새로 받은 숫자는 미로딩 상태의 `----·` 또는 기존 값에서 중간 숫자를 거쳐 최종 값까지 최대 4단계·600ms 안에 전환됩니다. 화면 안에서는 좌상단부터 시작하고 문자별 랜덤 지연을 주며, 화면 밖의 값은 즉시 반영해 스크롤 진입 때 다시 재생하지 않습니다. 캐시는 연결별 브라우저 메모리에만 있으며 브라우저 새로고침·연결 해제 시 초기화됩니다.
- `Conversation/Session menu`: 입력 툴바의 더보기 메뉴만 자동으로 열어 보여주는 예제. 기본·작업 중·중지·처리 대기·Claude 상태에서 세션 작업, 도구 목록과 하단 편집 옵션을 확인합니다. `AlwaysOpen`은 바깥 클릭·포커스 이동·Escape·항목 선택으로도 닫히지 않아 DevTools로 디자인을 확인하기에 적합합니다.
- `Conversation/EventCards`: 사용자 입력, 최종·중간 대화 응답, 작업 진행·완료, 파일 변경, 진단 메시지.
  `MermaidResponse`는 실제 대화 응답 안의 Mermaid 코드 블록을 다이어그램으로 표시합니다.
  `WideCodeBlock`은 일반·목록 안 코드블록의 가로 확장/복원을 보여줍니다. 복사 버튼 왼쪽 토글로 코드와 다이어그램 본문만 대화 패널 양끝까지 넓히며, 일반 본문과 툴바는 기존 폭과 위치를 유지합니다. 오른쪽 에디터 분할과 화면 크기 변경도 따릅니다.
  `QuestionHistoryInConversation`은 사용자·에이전트 대화 중간에 답변 완료된 Question 기록을 표시합니다.
  대화 응답과 같은 디자인의 카드 본문에서 질문별 탭을 선택해 질문과 기록된 답변을 확인합니다.
  Nox SlidingPages로 질문과 답변이 함께 슬라이드되며 카드와 각 영역의 높이는 유지됩니다.
  긴 내용은 내부에서 스크롤하고, 열어본 질문의 스크롤 위치는 탭을 다시 선택해도 유지됩니다.
  질문 번호 표시는 진행 중 QuestionCard와 공유합니다. 대화 기록은 번호만 중앙에 표시하는 버튼형 탭을 사용하며,
  다이얼로그의 완료 아이콘·탭 배경·둥근 모서리는 유지합니다. 질문·답변 본문은 다른 대화의 시작선에 정렬됩니다.
  `LongQuestionHistory`는 길이가 다른 질문·답변을 전환해도 이후 대화 위치가 유지되는 예제입니다.
- `Conversation/QuestionCard/LiveApprovals`: `Request approval` 또는 예제 안의 `Ctrl+Alt+N`으로 작업 허용 요청을 추가합니다.
  스택 배치·전진·보충·퇴장·가려짐 처리는 Nox `CardStack`을 사용합니다. 독립적인 재사용 예제는
  Nox Storybook의 `Components/CardStack`에 있습니다. 승인 응답과 요청 내용 렌더링은 cxz가 담당합니다.
  toolbar 더보기 왼쪽의 테두리가 있는 X·체크 그룹은 요청이 없어도 비활성 상태로 유지됩니다.
  요청이 없으면 아이콘과 반투명 테두리는 아주 희미하게 표시됩니다. 평소에는 배경 없이 어두운
  적색·녹색 아이콘을 표시하고, hover 시 밝아진 아이콘과 해당 색 배경을 표시합니다.
  요청 수·세로 구분선·X·체크 버튼이 같은 테두리 안에 표시됩니다. 요청 수는 고정 폭입니다.
  내부 높이는 그룹의 padding 안에서 늘어나도록 정렬하며, 별도의 버튼 높이 계산은 하지 않습니다.
  카운트는 폰트 기준 줄 높이를 사용해 같은 내부 영역의 중앙에 정렬합니다.
  수나 버튼에 hover하면 요청 내용만 담은 테두리와
  블러 배경의 작은 상자가 버튼 위 중앙에 뜨고, 마우스를 카드로 옮겨 내용을 읽거나 스크롤할 수 있습니다.
  키보드 포커스로도 열립니다. 처리된 요청은 투명도를 바꾸지 않고 작아지며 입력창 뒤로 들어갑니다.
  퇴장 카드가 앞 레이어를 유지하고, 다음 요청은 퇴장이 진행된 뒤 지연을 두고 앞으로 옵니다.
  그 뒤에 있던 카드는 지연을 두고 한 칸 전진합니다. 요청이 충분히 남아 있으면 같은 시점에
  다음 대기 카드가 맨 뒤로 올라와 빈 슬롯을 채웁니다. 화면에는 연속된 FIFO 요청을 표시합니다.
  앞으로 다가오는 동작은 일반 등장·퇴장보다 천천히 움직입니다. hover로 열 때는 앞 카드부터
  순서대로 올라옵니다. 닫을 때는 가장 뒤 카드부터 먼저 내려가고,
  가장 늦게 끝나는 앞 카드의 퇴장이 완료되면 전체 스택을 정리합니다.
  새 요청은 뒤에서 위로 이동합니다. 뒤 카드도 전체 내용을 유지하며, 앞 카드와 입력창에 가려지는
  영역만 움직임에 맞춰 잘라내어 배경이 중첩되지 않고 드러나는 본문도 비지 않습니다.
  퇴장 중에도 Question보다 위 레이어를 유지합니다. 움직임 줄이기 설정을 존중합니다. 일반 Question 폼은 기존 위치를 유지하며,
  자동으로 해결되는 허용 요청은 표시 대기 시간 안에 사라져 상자가 깜빡이거나 버튼이 활성화되지 않습니다.
- `Conversation/QuestionCard`: 라디오 선택, 주관식·Other 편집기, 단계별 탭과 Markdown 선택지 preview. FreeText, Steps, AsyncSteps, OptionPreviews, LongQuestions로 각 이벤트 형식과 전체 답변 제출을 확인합니다. 선택지 목록만 스크롤하고 Other와 footer는 고정됩니다. 질문 전환 시 가장 긴 질문에 맞춘 높이를 유지하며, 대화를 스크롤하면 카드가 입력창 뒤로 내려갔다가 마우스 접근이나 포커스로 복귀합니다. 상세 카드가 가려도 답변은 유지되며, 명시적 제출 또는 Cancel로만 종료합니다.
- `Conversation/Playground`: 메시지 입력과 전송부터 대화 카드 생성까지 이어지는 인터랙티브 미리보기.
- `Conversation/SessionSearch`: 현재 세션 검색 오버레이. `WithResults`는 검색창에 `network`가 입력되고 결과·선택 상태·본문 하이라이트·스크롤 마커가 표시된 상태로 시작하는 디자인 예제입니다. 결과 미리보기에서도 검색 판정에 기여한 문자열을 본문과 같은 형광색으로 강조하며, 원래 대소문자와 Markdown 표시 텍스트를 유지합니다. 닫기 버튼 옆에는 CompactNumber로 선택된 결과 순서와 전체 개수, 대소문자 구분·단어 단위 일치·정규식 토글을 표시합니다. 모든 검색 표시는 같은 식을 사용하며, 잘못된 정규식은 검색창에 오류를 표시하고 빈 문자열 일치는 제외합니다. `WithSearchOptions`는 세 토글을 켠 상태에서 대소문자·단어 경계·정규식의 조합과 한글 단어 검색을 확인하는 예제입니다. 결과에는 DateTime과 경과시간을 좌측 정렬합니다. 결과 목록도 검색 상자의 배경 블러를 공유하며, 마지막 항목 아래 여백과 바깥쪽 스크롤바를 유지합니다. `Default`에서는 단축키로 열고 입력·방향키·결과 선택을 확인하며, 본문 클릭이나 결과 이동으로 닫히지 않습니다. Close와 Escape로만 닫습니다. 로드된 예제 대화를 브라우저에서 검색하며, 응답 추가 시 결과 갱신도 확인할 수 있습니다.
- `Content/Markdown/Mermaid`: flowchart와 sequence diagram, 테마에 맞는 회색조, 원문 보기와 복사.
  `MermaidStreaming`은 미완성·잘못된 정의에서 복구, 응답 추가, 다이어그램 교체와 재마운트를 재현합니다.
  `MermaidUntrusted`는 HTML·스크립트 링크·페이지 CSS를 요청하는 정의에서도 앱의 보안 설정을 유지하는 예제입니다.
  `mermaid` 언어가 명시된 fenced code block만 자동으로 렌더링합니다. 다른 코드의 언어 선택과 문법 강조는 유지합니다.
  Mermaid는 npm 의존성으로 번들에 포함하되 다이어그램이 있을 때만 로드합니다. 외부 렌더링 서비스나 CDN은 사용하지 않습니다.
  [공식 렌더링 API](https://mermaid.js.org/config/usage.html)의 strict 설정을 사용하며, 생성된 SVG는
  이미지로 표시해 내부 스타일·ID를 대화 DOM과 분리합니다. 잘못된 정의와 로드 실패는 원문을 유지합니다.
  스트리밍 갱신은 모아서 처리하고, 취소된 요청은 화면을 덮어쓰지 않으며, 크기가 제한된 메모리 캐시로
  가상 스크롤 재마운트의 레이아웃 계산을 줄입니다. 컴포넌트의 CSS Module과 구현 코드가 크기·대기 시간·캐시 한도의 기준입니다.

## 계정 워크플로

`Accounts/Workflows`는 실제 계정 페이지와 폼, 로그인 패널, 세션 대화 컴포넌트를
연결합니다. 계정 RPC와 로그인 소켓은 예제마다 독립된 메모리 상태를 사용하며,
실제 Manager나 에이전트 제공자에 연결하거나 자격 증명을 저장하지 않습니다.
인증 코드에는 `demo-code`를 입력합니다. `Reset preview`는 예제의 계정, 인증 상태,
대화 초안과 진행 중 요청을 초기화합니다.

| Story                        | 확인할 흐름                                                                                         |
| ---------------------------- | --------------------------------------------------------------------------------------------------- |
| AddAccount                   | 빈 목록에서 Add account로 Codex/Claude 계정 추가, 중복 alias 오류                                   |
| AccountLogin                 | 공통 모달에서 Codex 공유 계정 로그인, 상태 새로고침, 취소·Escape·바깥 클릭으로 닫기와 로그아웃 확인 |
| LoginFailureAndRetry         | 제공자 오류 후 Retry login, 잘못된 코드 입력 후 재시도                                              |
| LoginConnectionClosed        | 로그인 도중 연결 종료와 새 연결로 재시도                                                            |
| SignedInAccount              | 로그인된 계정의 상태·사용량·연결된 세션, 로그아웃 확인·취소                                         |
| SessionCreationRequiresLogin | Claude 세션 생성의 로그인 필요 응답, 세션별 로그인, 동일 생성 키로 재시도                           |
| ConversationRequiresLogin    | Send 후 인증 만료로 실패한 턴, Log in again, 로그인 후 세션 재개와 다음 대화 제출                   |
| ExpiredSession               | 기록을 열었을 때 인증 만료 상태와 입력창 위 로그인 카드, 취소 후 복구 안내 유지                     |

Claude 인증 복구는 실제 앱처럼 이미 수신된 프롬프트를 자동 재전송하지 않습니다.
로그인 후에는 다음 메시지를 직접 전송합니다. `loginOutcome` control로 성공,
첫 시도 실패, 첫 연결 종료를 바꿀 수 있습니다. 이후 재시도는 정상 로그인으로 이어집니다.

예제와 fixture는 `ts/src/storybook/accounts/`에 있습니다. 로그인 소켓의 교체 범위는
`LoginTransportContext` 하위 트리로 제한됩니다. 전역 `WebSocket`을 바꾸지 않으며,
실제 앱은 provider가 없으므로 기존 Manager 소켓과 인증 정책을 그대로 사용합니다.

관련 브라우저 검사만 실행하려면 Storybook 정적 빌드 후 다음을 실행합니다.

```sh
cd ts
npx playwright test --config playwright.storybook.config.ts storybook-e2e/account-workflows.spec.ts
```

## Composer 영역과 Story 구성

**Composer**는 메시지 작성 영역 전체를 뜻합니다. `Compose`는 작성 동작을 가리킵니다.

| 영역                | 역할                                                                  | 코드                                                                      |
| ------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| Composer toolbar    | Stop, 경과 시간, 세션 메뉴, Terminal, Latest, Send를 배치하는 상단 바 | `.composer-toolbar`                                                       |
| Composer editor     | 줄번호, Markdown, 코드블록, 붙여넣기 chip을 포함하는 입력창           | `ComposerEditor`, `.composer-editor`; 입력창 컨테이너는 `.composer-input` |
| Composer status bar | Model, Effort, usage, context를 표시하는 하단 영역                    | `.composer-meta`                                                          |
| Composer surface    | Toolbar와 입력창을 감싸는 박스. Status bar는 이 박스 밖에 배치        | `.composer-wrapper`                                                       |
| Composer            | Toolbar, Editor, Status bar를 포함하는 전체 영역                      | `ConversationComposer`, `form.composer`                                   |

`Conversation/Composer`는 영역별 단독 Story가 아니라 **전체 Composer의 상태별 Story**입니다.
`Empty`, `Markdown`, `CodeBlock`, `PasteChip`, `Commands`, `PendingSend`, `Working`, `StoppedSession`을 제공합니다.
세션 메뉴에서 정지/재개, 재시작 확인, 모의 Purge 계획과 상세 정보를 확인할 수 있습니다.
Details의 Title·Alias를 클릭하면 주변이 어두워지고 값 위치 그대로 편집됩니다.
Storybook의 관리 동작은 미리보기 내부 상태만 변경하며 실제 세션이나 파일을 변경하지 않습니다.
입력창은 별도 `ComposerEditor` 컴포넌트이지만, 이 Story에서는 상단 바와 하단 상태 영역을
함께 표시합니다. Toolbar와 Status bar는 현재 `ConversationComposer` 내부 영역입니다.
이 영역에 들어가는 버튼, 선택 메뉴, 타이머 같은 재사용 컴포넌트는 Nox Storybook에서
별도 Story로 확인할 수 있습니다. cxz의 세션 검증 규칙을 사용하는 `Components/EditableValue`와
실행·실패·재시도 정책을 가진 `Components/ConfirmationDialog`는 앱 연동 예제로 남깁니다.
작성·전송·응답 생성이 이어지는 흐름은
`Conversation/Playground`에서 확인합니다.

Playground에서 메시지를 작성하고 화살표 버튼이나 Ctrl+Enter로 전송합니다.
전송 확인 대기 동안 입력창이 잠기고, 확인 후 전송 애니메이션과 함께 사용자 카드가 생성됩니다.
이어서 작업 중 glow·타이머와 모의 에이전트 응답이 표시됩니다. Stop 버튼 두 번이나
Esc 두 번으로 모의 응답을 정지할 수 있습니다. Reset preview는 기록과 대기 중인 응답을 초기화합니다.
긴 대화와 느린 전송 확인 상태도 별도 스토리로 제공합니다.

`RapidResponses`는 긴 기록에서, `RapidResponsesFromEmpty`는 빈 대화에서 시작합니다.
메시지를 Send하면 중간 대화 응답과 작업 시작·출력·완료가 빠르게 이어지고 최종 응답으로 끝납니다.
Controls의 `burstUpdates`와 `burstIntervalMs`로 발생 횟수와 간격을 조절합니다.
미리보기의 추적 상태와 `Bottom gap`으로 맨 아래를 계속 따라가는지 확인할 수 있습니다.
진행 중 위로 스크롤해 기록을 읽거나 Latest로 추적을 재개할 수 있으며, Stop과 Reset은 발생을 중단합니다.

여러 줄 텍스트를 붙여넣으면 chip이 만들어지고, chip을 누르면 미리보기 카드가 열립니다.
전송된 카드에는 chip이 가리키는 원문이 들어갑니다. 작업 카드를 더블클릭하면
카드 아래에 Input/Output/Result 탭과 Monaco 에디터가 열립니다.
`Conversation/EventCards/FoldedActivityStack`에서 접힌 작업 카드의 겹침,
hover·키보드 focus로 펼침, 상세보기와 안정적인 대화 배치를 확인할 수 있습니다.
대화 본문을 누르면 작업 상세가 닫힙니다. 질문 카드는 답변 제출 또는 Cancel로 닫습니다.

이 미리보기는 합성 protobuf 이벤트를 사용합니다. 실제 cxz 서버에 요청하거나 계정 quota를
소비하지 않으며, 터미널 연결은 비활성화되어 있습니다. UI와 전송 애니메이션은 실제 앱과
같은 컴포넌트를 사용합니다.

상단 툴바에서 `dark` / `light` 테마와 `en` / `ko` 언어를 전환합니다.
테마는 코드 에디터에도 적용되며, 툴바 전환은 저장된 settings 파일을 변경하지 않습니다.
Controls에서 props를 바꾸거나 컴포넌트를 직접 클릭해 동작을 확인합니다.
Docs에서 컴포넌트의 props와 여러 상태를 함께 확인합니다.

스토리는 `ts/src/**/*.stories.tsx`, 공통 설정은 `ts/.storybook/`에 있습니다.
새 cxz 컴포넌트는 같은 위치에 `컴포넌트명.stories.tsx`를 추가하면 자동으로 표시됩니다.
Nox 컴포넌트의 기본 상태·props 예제를 cxz의 `src`에 복제하지 않습니다.
앱 Storybook에는 cxz 데이터·검증·설정·RPC를 연결하거나 제품 화면 안에서 조합하는 예제를 둡니다.
미리보기 전용 프레임과 합성 데이터는 `ts/src/storybook/`에 있으며 앱 번들에서는 사용하지 않습니다.

정적 빌드와 브라우저 검증:

```sh
cd ts
npm run build-storybook
npm run test:storybook
```

빌드 결과는 Git에서 제외된 `ts/storybook-static/`에 생성됩니다.
브라우저 검증은 정적 빌드를 별도 포트에서 서빙하며 입력·전송·chip·상세 탭·질문 응답·세션 선택을 확인합니다.

## 독립 UI 패키지

다른 프로젝트에서 사용할 기본 컴포넌트는 `ts/packages/nox` Git submodule의 `@lesomnus/nox`를 사용합니다. 소스 수정과 의존성 커밋 관리 방법은 [Nox development](nox-development.md)를 참조하세요. 각 컴포넌트의 스타일은 옆에 있는 CSS Module이 소유하고 공통 디자인 값은 패키지의 `tokens.css`가 관리합니다. 앱의 `style.css`에는 세션·대화·설정 페이지 배치와 공용 컴포넌트를 그 배치에 맞추는 스타일을 남깁니다. 번역은 UIProvider로 주입하고 Monaco 에디터의 설정·테마는 props로 전달합니다.

```sh
cd ts
npm run storybook:ui
```

별도의 UI Storybook은 앱 전역 CSS와 설정·언어 저장소 없이 패키지를 검증합니다. 앱 Storybook은 실제 제품 화면 안에서의 조합을 검증합니다. 패키지 설치·테마·번역·에디터 사용법은 [UI README](https://github.com/lesomnus/nox/blob/main/README.md)를 참조하세요. 배포용 CSS와 타입 선언은 빌드 결과이며 git에 저장하지 않습니다.

Nox Storybook은 <http://localhost:6008>에서 열립니다. `ts/.storybook-ui`가
`ts/packages/nox/src/**/*.stories.tsx`를 등록하므로 Button, Checkbox, Switch,
ValueMenu, FieldLabel, IdentifierValue, DateTime, ElapsedTime, SettingSlider,
SegmentedControl, Card, Drawer, SourceEditor 등은 여기서 확인합니다.
cxz의 `ts/.storybook/main.ts`에는 Nox 소스 경로를 직접 등록하지 않습니다.

기본 컴포넌트 브라우저 검증은 `ts/ui-e2e`에서 Nox 카탈로그를 대상으로 실행합니다.
제품 연동 검증은 `ts/storybook-e2e`에서 cxz 카탈로그만 사용합니다. 한 테스트에서
두 카탈로그의 스토리를 오가지 않습니다.

```sh
cd ts
npm run build-storybook:ui
npm run test:ui
```

`Components/ScrollArea`는 중앙 레일·핸들·본문 배경색의 그림자를 가진 재사용 세로 스크롤입니다.
`LongList`, `ShortOverflow`, `NoOverflow`, `GrowingContent`에서 내용 길이에 따른 레일 변화,
hover·포커스·드래그, 키보드 이동과 `viewportRef`를 통한 위치 이동을 확인합니다.
앱의 `PanelScroll`은 이 컴포넌트에 번역과 패널 배치를 연결하는 래퍼입니다.

소스의 역할별 분류와 파일 배치 기준은 [웹 소스 구조](web-structure.md)에 정리되어 있습니다.

`Conversation/Composer`의 `FileUploads`, `FailedUpload`는 파일·폴더 드롭과
toolbar의 파일·폴더 선택, 큰 붙여넣기, 업로드 대기 중 편집·전송 차단,
chip 상세·삭제·재시도를 보여줍니다. Storybook은 업로드 응답만 흉내 내며 파일을
서버에 저장하지 않습니다. 실제 업로드 연결은 WASM sandbox의 메모리 파일과
인증된 gateway 브라우저 fixture에서 검증합니다.
