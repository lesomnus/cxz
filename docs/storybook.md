# Storybook

앱의 실제 React 컴포넌트와 CSS를 서버 연결 없이 확인합니다.

```sh
cd ts
npm ci
npm run storybook
```

브라우저에서 <http://localhost:6006>을 엽니다. 컨테이너나 원격 개발 환경에서는
6006 포트를 로컬로 포워딩합니다.

- `Components`: Button, AgentBrand, SegmentedControl, SettingSlider, ValueMenu
- 각 컴포넌트의 기본 상태, 비활성 상태, 상속 값 등은 왼쪽 스토리 목록에서 선택합니다.
- 상단 툴바에서 `dark` / `light` 테마와 `ko` / `en` 언어를 전환합니다.
- Controls에서 props를 바꾸거나 컴포넌트를 직접 클릭해 동작을 확인합니다.
- Docs에서 컴포넌트의 props와 여러 상태를 함께 확인합니다.

스토리는 `ts/src/*.stories.tsx`, 공통 설정은 `ts/.storybook/`에 있습니다.
새 컴포넌트는 같은 위치에 `컴포넌트명.stories.tsx`를 추가하면 자동으로 표시됩니다.
선택 메뉴와 슬라이더 스토리는 `useArgs`로 선택 값을 Controls와 동기화합니다.

정적 빌드:

```sh
cd ts
npm run build-storybook
```

결과는 Git에서 제외된 `ts/storybook-static/`에 생성됩니다.
