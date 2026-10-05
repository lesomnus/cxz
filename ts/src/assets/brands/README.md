# 에이전트 로고

2026-10-05 확인. 공식 SVG 원본은 받은 바이트 그대로 보관하고,
아래에 표시용 파생 파일을 구분한다.
이 파일들은 각 회사의 상표이며, 프로젝트의 자체 로고가 아니다.
다운로드 가능하다는 사실이 모든 용도에 대한 사용 승인을 뜻하지 않는다.
OpenAI Cookbook 원본의 MIT 저작권 고지와 두 상표의 출처는
[third-party-notices.txt](../../../public/third-party-notices.txt)에 보관하며,
Vite가 sandbox와 production 배포 파일에 함께 복사한다. MIT 고지가
상표 사용 조건을 대체하지는 않는다.

| 파일                     | 공식 출처 / 원본                                                                                                                                                                                                 | UI 표시                                                                   |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `openai-blossom.svg`     | [OpenAI Cookbook 원본](https://github.com/openai/openai-cookbook/blob/0eac1447d4e24d06e47c459ca5e98f248b9413cf/examples/agents_sdk/deployment_manager/frontend/src/openai-logomark.svg)                          | Codex 에이전트 식별용 OpenAI Blossom, 원본 `currentColor`를 흰색으로 표시 |
| `claude-one-color.svg`   | [Anthropic Newsroom](https://www.anthropic.com/news)의 [공식 press kit](https://anthropic.com/press-kit), `Anthropic media resources/Anthropic logos/Claude logos/1 Claude logo/SVG/Claude logo - One-color.svg` | 분리 전 공식 원본 보관                                                    |
| `claude-spark-white.svg` | 위 공식 원본에서 흰색 Spark path만 분리한 파생 파일                                                                                                                                                              | 워드마크 없이 흰색 별 심볼만 표시                                         |

공개 OpenAI 브랜드 페이지와 `openai/codex` 저장소에서 배포용 Codex 전용
SVG를 확인하지 못했으므로 OpenAI Blossom을 사용한다. Codex 전용 로고라고
표기하지 않는다. Claude Spark 단독 SVG는 Clay 색상으로만 제공된다.
`Slate`/`Ivory` 조합에도 Clay 색상이 포함되어 있다. 사용자 요청으로 흰색
`One-color` 조합에서 두 번째 path(Spark)를 분리하고 워드마크를 제외했다.
Spark의 path 데이터와 흰색 fill은 그대로 유지하고, 캔버스만 원본에서 심볼이
위치한 `0 0 2000 2000`으로 제한한다. 공식 원본은 삭제하거나 수정하지 않는다.
이 파생 파일은 공식 배포된 별 단독 모노크롬 variant가 아니다.

## 사용 가이드

- [OpenAI Design Guidelines 및 Marks usage terms](https://openai.com/brand/):
  OpenAI 서비스와 직접 관련된 위치에서 원본을 사용하고, 형태·색상·비율을
  임의 변경하거나 효과를 추가하지 않는다. 여백을 유지하고 앱 자체 브랜드보다
  두드러지게 사용하거나 제휴·보증을 암시하지 않는다. 이 SVG의 색상 상속은
  공식 저장소 원본에 있는 `currentColor` 동작을 그대로 이용한다. 별도의 Codex
  모노크롬 브랜드 키트가 제공된다는 근거는 확인하지 못했다.
- [Anthropic Trademark Guidelines](https://www.anthropic.com/legal/trademark-guidelines):
  회사가 허용한 용도와 사전에 승인한 자료에만 사용할 수 있다고 명시한다.
  색상·폰트·비율 변경을 금지하고, 읽을 수 있는 배경과 주변 여백을 요구한다.
  모노크롬 전환 규칙은 별도로 확인하지 못했지만 공식 kit 자체가 흰색
  `One-color` SVG를 제공한다. 별만 분리하는 사용까지 승인되었다는 근거는
  확인하지 못했으며, 파생 파일 사용도 사전 승인 검토 대상이다.

현재 구현은 서비스 식별 위치(대화 머릿말, 세션 헤더, 모델이 없는 목록 항목)에
작게 표시한다. 접근성 이름과 브라우저 tooltip은 Codex/Claude로 유지하며,
모델 이름과 실제 대화 본문은 변경하지 않는다. 심볼 비율 유지, 필터 없음.
높이는 14px로 맞춰 기존 텍스트 머릿말과 대화 행의 높이를 유지한다.
SVG 안의 script, 이벤트 핸들러, 외부 참조 및 embedded image가 없음을 확인했다.

공개 배포 검토 시 Anthropic의 사전 승인 조건과 OpenAI의 용도 조건을 확인해야
한다. 이 저장소에는 Anthropic 사용 승인을 받았다는 기록이 없다.
문의 창구는 각각 `marketing@anthropic.com`, `partnercomms@openai.com`이다.

## 원본 검증

| 파일                            | SHA-256                                                            |
| ------------------------------- | ------------------------------------------------------------------ |
| `openai-blossom.svg`            | `a41e8e53b14ef686319ed1529066d6a3391aca77d7f6eaaa9f0783acc717375f` |
| `claude-one-color.svg`          | `40b1279a4a83bf324de28ccd0e39fffa732462df42cc856d9d3dbb6b6917799a` |
| `claude-spark-white.svg` (파생) | `27e2fab1d44510c2d9787aa0eb817c3492198c0f7864f7123fcd10a1c022a1d7` |

SVG 원본을 포맷하거나 최적화하지 않는다. 교체 시 공식 출처, 사용 조건과
해시를 함께 갱신한다.
