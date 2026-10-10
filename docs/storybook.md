# Storybook

앱의 실제 React 컴포넌트와 CSS를 서버 연결 없이 확인합니다. 기본 언어는 영어입니다.

```sh
cd ts
npm ci
npm run storybook
```

브라우저에서 <http://localhost:6006>을 엽니다. 컨테이너나 원격 개발 환경에서는
6006 포트를 로컬로 포워딩합니다.

- `Components`: 버튼, 에이전트 로고, 설정 드롭다운·슬라이더·분할 선택, floating card, 타이머와 Monaco 상세 탭.
- `Settings/SettingField`: 제목·설정 ID·요약·컨트롤·선택적 상세설명을 배치하는 공통 설정 항목.
- `Editor/SourceEditor`: 파일 미리보기와 편집 가능한 settings JSON.
- `Sessions/Panel`: 실제 세션 카드, 선택 하이라이트, 상태 indicator, 프로젝트 접기, 긴 목록의 스크롤.
- `Conversation/Composer`: Markdown, 코드블록, 붙여넣기 chip, 명령어 제안, 전송 대기, 작업 중 glow, 모델·effort와 usage 표시.
- `Conversation/EventCards`: 사용자 입력, 최종·중간 대화 응답, 작업 진행·완료, 파일 변경, 진단 메시지.
- `Conversation/QuestionCard`: 라디오 선택, 여러 줄 Other 답변, 명시적 제출, 상세 카드에 가려지는 질문 카드.
- `Conversation/Playground`: 메시지 입력과 전송부터 대화 카드 생성까지 이어지는 인터랙티브 미리보기.

## Composer 영역과 Story 구성

**Composer**는 메시지 작성 영역 전체를 뜻합니다. `Compose`는 작성 동작을 가리킵니다.

| 영역                | 역할                                                           | 코드                                                                      |
| ------------------- | -------------------------------------------------------------- | ------------------------------------------------------------------------- |
| Composer toolbar    | Stop, 경과 시간, Terminal, Latest, Send를 배치하는 상단 바     | `.composer-toolbar`                                                       |
| Composer editor     | 줄번호, Markdown, 코드블록, 붙여넣기 chip을 포함하는 입력창    | `ComposerEditor`, `.composer-editor`; 입력창 컨테이너는 `.composer-input` |
| Composer status bar | Model, Effort, usage, context를 표시하는 하단 영역             | `.composer-meta`                                                          |
| Composer surface    | Toolbar와 입력창을 감싸는 박스. Status bar는 이 박스 밖에 배치 | `.composer-wrapper`                                                       |
| Composer            | Toolbar, Editor, Status bar를 포함하는 전체 영역               | `ConversationComposer`, `form.composer`                                   |

`Conversation/Composer`는 영역별 단독 Story가 아니라 **전체 Composer의 상태별 Story**입니다.
`Empty`, `Markdown`, `CodeBlock`, `PasteChip`, `Commands`, `PendingSend`, `Working`을 제공합니다.
입력창은 별도 `ComposerEditor` 컴포넌트이지만, 이 Story에서는 상단 바와 하단 상태 영역을
함께 표시합니다. Toolbar와 Status bar는 현재 `ConversationComposer` 내부 영역입니다.
이 영역에 들어가는 버튼, 선택 메뉴, 타이머 같은 재사용 컴포넌트는 `Components`에서
별도 Story로 확인할 수 있습니다. 작성·전송·응답 생성이 이어지는 흐름은
`Conversation/Playground`에서 확인합니다.

Playground에서 메시지를 작성하고 화살표 버튼이나 Ctrl+Enter로 전송합니다.
전송 확인 대기 동안 입력창이 잠기고, 확인 후 전송 애니메이션과 함께 사용자 카드가 생성됩니다.
이어서 작업 중 glow·타이머와 모의 에이전트 응답이 표시됩니다. Stop 버튼 두 번이나
Esc 두 번으로 모의 응답을 정지할 수 있습니다. Reset preview는 기록과 대기 중인 응답을 초기화합니다.
긴 대화와 느린 전송 확인 상태도 별도 스토리로 제공합니다.

여러 줄 텍스트를 붙여넣으면 chip이 만들어지고, chip을 누르면 미리보기 카드가 열립니다.
전송된 카드에는 chip이 가리키는 원문이 들어갑니다. 작업 카드를 더블클릭하면
카드 아래에 Input/Output/Result 탭과 Monaco 에디터가 열립니다.
`Conversation/EventCards/FoldedActivityStack`에서 접힌 작업 카드의 겹침,
hover·키보드 focus로 펼침, 상세보기와 안정적인 대화 배치를 확인할 수 있습니다.
대화 본문을 누르면 작업 상세가 닫힙니다. 질문 카드는 답변 제출 또는 Deny로 닫습니다.

이 미리보기는 합성 protobuf 이벤트를 사용합니다. 실제 cxz 서버에 요청하거나 계정 quota를
소비하지 않으며, 터미널 연결은 비활성화되어 있습니다. UI와 전송 애니메이션은 실제 앱과
같은 컴포넌트를 사용합니다.

상단 툴바에서 `dark` / `light` 테마와 `en` / `ko` 언어를 전환합니다.
테마는 코드 에디터에도 적용되며, 툴바 전환은 저장된 settings 파일을 변경하지 않습니다.
Controls에서 props를 바꾸거나 컴포넌트를 직접 클릭해 동작을 확인합니다.
Docs에서 컴포넌트의 props와 여러 상태를 함께 확인합니다.

스토리는 `ts/src/*.stories.tsx`, 공통 설정은 `ts/.storybook/`에 있습니다.
새 컴포넌트는 같은 위치에 `컴포넌트명.stories.tsx`를 추가하면 자동으로 표시됩니다.
선택 메뉴와 슬라이더, 입력창 스토리는 `useArgs`로 선택 값을 Controls와 동기화합니다.
미리보기 전용 프레임과 합성 데이터는 `ts/src/storybook/`에 있으며 앱 번들에서는 사용하지 않습니다.

정적 빌드와 브라우저 검증:

```sh
cd ts
npm run build-storybook
npm run test:storybook
```

빌드 결과는 Git에서 제외된 `ts/storybook-static/`에 생성됩니다.
브라우저 검증은 정적 빌드를 별도 포트에서 서빙하며 입력·전송·chip·상세 탭·질문 응답·세션 선택을 확인합니다.
