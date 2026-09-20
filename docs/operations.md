# TUI 운영·배포

## 설정과 진단

```sh
cxz account add codex work-codex
cxz config set codex-model MODEL_ID
cxz config set claude-model MODEL_ID_OR_ALIAS
cxz config show
cxz config unset codex-model
cxz session new --model MODEL_ID .
cxz manager doctor
cxz project logs                         # manager stdout/stderr, last 100 lines
cxz project logs --tail 200 PROJECT       # devcontainer provisioning log
cxz version
```

설정은 해당 `--state`의 `settings.jsonc`에 저장한다(Linux 파일 모드 0600).
Windows 기본 경로는 `%USERPROFILE%\.local\state\cxz\settings.jsonc`이다.
디렉터리 우선순위는 `--state` > `CXZ_STATE` > `XDG_STATE_HOME/cxz` > 홈 디렉터리 기본값이다.
처음 `cxz edit`를 열면 연결·모델·파일 공유·Docker 예시가 비활성 주석으로 들어간다.
`//`, `/* ... */` 주석과 마지막 쉼표를 지원하며, CLI 설정 변경도 관련 없는 주석을 보존한다.
`cxz edit` 실행 시 바이너리에 포함된 스키마를 같은 디렉터리의 `settings.schema.json`으로
생성·갱신한다. 첫 설정 파일에는 `"$schema": "./settings.schema.json"`이 들어가며,
기존 파일에도 이 필드가 없으면 편집 시 추가한다. 사용자가 지정한 스키마는 유지한다.
VS Code는 `.jsonc`를 JSON with Comments로 인식하므로 별도의 파일 연결 설정이 필요 없다.
설정은 `settings.jsonc` → `settings.jsonm` → `settings.json` 순서로 찾는다.
이전 파일을 읽은 경우 다음 저장부터 `settings.jsonc`로 저장하고 원본은 백업으로 남긴다.
인증정보와 권한 정책은 이 설정에 넣을 수 없다. CLI 명시 값 > client 설정 > vendor 기본값 순서다.
새 세션의 agent는 선택한 Account로 결정한다. `project up`은 기존 Account/vendor/model을 유지한다.
모델은 세션 생성 시 고정된다. 기존 대화의 모델을 몰래 바꾸지 않으며 변경은 명시적으로
기존 세션을 stop한 뒤 새 세션을 만든다. 사용 가능한 모델은 vendor 계정에 따라 다르다.

`manager doctor`는 설정, Docker, 설치 locator, manager 소유권, project inventory와 세션 RPC를
검사한다. 실패 시 상세 표와 비영 종료 코드를 반환한다(`--format json` 지원). 유료 요청이나 자격증명 읽기는
하지 않으므로 **인증 성공을 보장하는 검사가 아니다**. 정상적으로 down한 프로젝트는
오류가 아니다. 로그는 hook이 출력한 비밀/경로가 포함될 수 있으므로 공개 공유 전에 검토한다.

TUI는 vendor 진단과 실패 payload를 표시한다. 인증 실패 의심 메시지는 로그인 경로를
안내하지만 자동 로그인/토큰 갱신/프롬프트 재전송은 하지 않는다.

```sh
cxz account login ACCOUNT
cxz project up PROJECT
```

## 중간 실패와 복구

Account 등록·로그인·계정 격리 범위는 [Account 문서](accounts.md)를 참고한다.

`up/new/recreate`는 프로젝트 등록·컨테이너 준비/삭제 전에 계정 필요 여부,
등록된 계정의 agent/backend, 활성 세션 충돌, 기존 세션의 모델 불변 조건을 확인한다.
기존 세션이 있으면 `project up`은 그 계정을 유지한다. 없으면 대화형 터미널에서는 Account를
선택하고, 비대화형에서는 `--account ACCOUNT`가 없을 때 즉시 실패한다.
`--trust-config`는 이 조건을 건너뛰지 않는다. 로그인용 prepare-only는 계정 없는
세션을 만들지 않으므로 세션 계정 선택을 요구하지 않는다.
사전 조회 실패를 빈 프로젝트로 간주하지 않으며, 준비 후에도 세션 상태를 다시 확인한다.
실제 vendor 인증의 유효성 등 프로젝트 런타임이 필요한 검사는 준비 이후 수행한다.

devcontainer 신뢰 검사로 차단되면 해당 설정 파일명과 JSON Pointer 경로·이유를
출력한다. 예: `/initializeCommand`(호스트 초기화 명령),
`/services/dev/privileged`(privileged 컨테이너), `/runArgs/0`(배열 첫 항목).
여러 원인은 모두 일정한 순서로 표시한다. 명령·환경변수·mount 원문 값은 출력하지 않는다.
Compose 파일도 같은 진단을 제공한다. 해당 설정을 직접 확인한 뒤 신뢰한다면
`cxz project up --trust-config PROJECT`로 재시도한다. 탐지는 문자열 참조도 포함하는 보수적인
검사이며, 이 메시지가 실제 권한 사용을 증명하거나 완전한 보안 검사를 뜻하지는 않는다.

- `manager install` 실패 후 같은 client state로 재실행한다. owner/volume 이름이 유지된다.
  같은 이미지·workspace root의 기존 설치는 준비 상태를 다시 확인하고, 정지된 manager는
  시작한다. 이미지/root 변경은 명시적인 `manager install --recreate`가 필요하다.
- `project up` 준비 단계는 project manifest에 저장된다. `project ls`의 `provision_state`,
  `provision_step`, `provision_attempt`와 `manager logs PROJECT`로 실패 위치를 확인한다.
  manager가 중간에 죽으면 다음 시작에서 interrupted로 표시한다.
- 같은 `project up`을 재시도하면 소유 label과 실제 리소스를 재확인한다. 동시 요청은 프로젝트별로
  직렬화한다. 사용자 devcontainer hook은 다시 실행될 수 있어 멱등하게 작성해야 한다.
- 외부 컨테이너는 편입하지 않는다. 중복 소유 컨테이너도 임의로 하나를 선택하지 않는다.
- manager 재시작은 agent 재시작이 아니다. 프로젝트 컨테이너 소실은 같은 세션을 새 run으로
  명시적으로 resume하는 복구다. 이전 승인과 prompt를 자동 재전송하지 않는다.

## 릴리스와 설치

현재 대상은 **Linux amd64/arm64**다. arm64는 cross-build 검증과 실제 실행 검증을 구분한다.
Go 버전은 `go.mod`를 따른다. `bash scripts/release.sh vX.Y.Z`가 `dist/`에 아키텍처별
tar.gz, raw binary, `SHA256SUMS`를 만든다. 압축 메타데이터는 고정하지만 빌드 입력 전체의
재현성을 보장하는 공급망 attestation을 대체하지는 않는다.

GitHub Actions CI는 test/race/vet/proto 재생성/cross-build를 실행한다. 버전 tag push는
같은 검사를 거쳐 GHCR multi-platform 이미지와 GitHub Release를 게시한다. prerelease tag는
prerelease로 게시하고 `latest` tag는 만들지 않는다. Actions는 commit SHA로 고정했다.
workflow 구현은 [GitHub 공식 이미지 배포 안내](https://docs.github.com/en/actions/tutorials/publish-packages/publish-docker-images)를 따른다.
게시에는 저장소 Actions 및 `contents:write`, `packages:write`가 필요하다. 최초 GHCR package가
비공개이면 공개 배포를 위해 소유자가 가시성을 설정해야 한다. 로컬 빌드 성공만으로 원격
게시 완료를 뜻하지 않는다.

게시된 release의 해당 아키텍처 tar.gz와 SHA256SUMS를 내려받고 체크섬을 비교한 뒤
빈 임시 디렉터리에 압축을 푼다. 검증한 `cxz`를 사용자 PATH에 설치한다. manager-image.txt의
digest를 사용하면 tag 변경과 무관하게 이미지를 고정할 수 있다.

```sh
cxz install --image ghcr.io/lesomnus/cxz:vX.Y.Z --workspace-root /absolute/projects
cxz manager doctor
cxz session new --account work-codex .
```

호스트와 Docker engine 아키텍처가 다르면 `--image`로 대응 platform 이미지를 사용한다.
현재 binary로 manager를 빌드하는 기본 설치는 플랫폼이 다르면 사전에 거부한다.
원격 Docker bind는 engine 관점의 경로다. 이 개발 환경에서는 `/workspaces/...`를 사용한다.

## 업데이트와 롤백

```sh
cxz manager update ghcr.io/lesomnus/cxz:vX.Y.Z
cxz manager doctor
cxz manager rollback
```

업데이트는 명시한 버전/digest만 사용하고 다운로드가 실패하면 기존 manager를 제거하지 않는다.
client locator에 이전 이미지가 기록된다. rollback은 그 이미지로 manager를 교체하며
workspace, project container, named volumes를 삭제하지 않는다. client binary는 별도로
이전 검증본을 보관/복원한다. 프로젝트의 이미 실행 중인 runtime/supervisor는 그대로 살아 있어
새 기능을 적용하려면 세션을 마무리한 뒤 명시적인 project recreate가 필요할 수 있다.

이 롤백은 **manager 이미지 롤백**이지 데이터 되감기/외부 백업이 아니다. 향후 비호환 데이터
마이그레이션 전에는 별도 백업과 호환성 확인이 필요하다. `manager uninstall`도 named volume을
지우지 않으며 공유 엔진에서 prune을 사용하면 안 된다.

## 인수 검사

1. 빈 client state에서 install, 동일 install 재호출, doctor.
2. 소유 workspace new, vendor 선택, 사용자 login, 대화/승인/거절/중단/재접속.
3. 준비 중 manager 강제 종료, 상태 checkpoint 확인, 동시 up 재시도 시 중복 방지.
4. manager 교체/rollback 후 기존 session run 유지.
5. down/up 후 vendor ID와 과거 대화 기억 확인. Claude와 Codex를 별도로 판정한다.

자동 재현: `owned-boundaries.mjs`, `owned-provision-retry.mjs`,
`owned-install-update.mjs`, `owned-session-live.mjs`, `owned-recovery.mjs`. 인증 검사는 사용자 로그인 또는 명시적으로
허용된 일회성 access token이 있어야 한다. 인증 검사를 건너뛴 결과를 통과로 적지 않는다.

Claude 반복 복구 문제를 도구 승인/중단과 분리하려면 **새 disposable 프로젝트에 로그인한 뒤**
`CXZ_PROBE_MEMORY_ONLY=1 node scripts/probes/owned-session-live.mjs STATE PROJECT claude`를
실행한다. 일반 사용자 메시지의 프로젝트 별칭만 기억하는 최소 대화로 두 번 복구한다.
실패 시 vendor ID 유지 여부와 `reasoning_extraction` 응답 관찰 여부만 공개 요약에 기록한다.
해당 응답 관찰만으로 원인이 vendor에만 있다고 단정하지 않으며 안전장치는 끄지 않는다.
