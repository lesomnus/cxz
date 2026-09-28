# 사용자 기본 devcontainer 템플릿

Linux 호스트에서 `settings.jsonc` 옆에 `devcontainer/devcontainer.json`을 만들면,
프로젝트 자체 devcontainer 설정이 없는 경우 이 템플릿을 사용한다. 별도 설정 항목은 없다.
기본 경로는 `~/.local/state/cxz/devcontainer/`이며 `--state`, `CXZ_STATE`,
`XDG_STATE_HOME`으로 cxz 상태 경로를 변경했다면 해당 경로를 사용한다.

```text
<cxz 사용자 상태 경로>/
├── settings.jsonc
└── devcontainer/
    ├── devcontainer.json
    ├── Dockerfile          # 선택
    └── scripts/           # 선택
```

예를 들어 `devcontainer.json`을 다음과 같이 작성한다. JSONC 주석과 trailing comma도 지원한다.

```json
{
  "name": "${cxz:projectName}",
  "image": "mcr.microsoft.com/devcontainers/base:bookworm",
  "remoteUser": "vscode"
}
```

선택 우선순위는 명시적 `--config`, 프로젝트의 자동 탐색 설정
(`.devcontainer/devcontainer.json`, `.devcontainer.json` 등), 사용자 템플릿,
내장 기본값 순이다. 사용자 템플릿은 프로젝트 설정과 병합하지 않는다.
프로젝트에 파일을 만들거나 Git 저장소를 변경하지 않는다.

`cxz up WORKSPACE` 또는 `project up/new/recreate`, `session new`를 호스트에서 실행하면
템플릿을 읽어 Manager에 저장한다. TUI는 마지막으로 전달한 사본을 사용한다.
리모트 연결에서는 컨테이너를 실행하는 Linux 호스트 사용자의 설정을 기준으로 한다.
Windows/리모트 클라이언트에 디렉터리를 만드는 것만으로 호스트에 적용되지 않는다.

## 프로젝트 이름 변수

`${cxz:projectName}`은 **사용자 템플릿의 devcontainer.json 문자열 값**에서 치환한다.
이름 선택은 다음 순서를 따른다.

1. Git `origin` URL의 마지막 레포 이름에서 `.git` 제거 (`org/my-app.git` → `my-app`).
2. origin이 없는 Git 저장소는 저장소 루트 디렉터리명.
3. Git 저장소가 아니거나 Git 메타데이터를 읽을 수 없으면 대상 디렉터리명.
4. `/`처럼 디렉터리 이름이 없으면 `workspace`.

워크트리도 origin을 읽을 수 있으면 같은 레포 이름을 사용한다. 네트워크 조회는 하지 않는다.
프로젝트 ID나 별칭은 변경하지 않는다. 따옴표를 포함한 이름도 JSON 문자열로 안전하게 인코딩한다.
기존 `${localWorkspaceFolder}` 같은 devcontainer 변수는 그대로 보존한다.
Dockerfile·Compose·스크립트 내용과 프로젝트 자체 설정에는 cxz 변수를 치환하지 않는다.

## 부속 파일과 적용 시점

Dockerfile, Compose 파일, 스크립트는 템플릿 디렉터리 안에 넣는다.
파일들의 상대 경로와 실행 권한은 보존한다. 전체 크기는 1 MiB, 파일 수는 256개까지이며,
심볼릭 링크는 지원하지 않는다. 템플릿 디렉터리가 없으면 내장 기본값으로 돌아가지만,
디렉터리가 있는데 devcontainer.json이 없거나 설정이 잘못된 경우에는 오류를 표시한다.

Manager는 프로젝트별 `projects/<ID>/templates/<내용 해시>/`에 사본을 만들고,
컨테이너 재생성 전에 설정과 기존 권한 신뢰 규칙을 검사한다. 원본 템플릿은 수정하지 않는다.
이미지/Dockerfile 템플릿에서 생략한 `workspaceFolder`는 `/workspaces/<대상 디렉터리명>`,
`workspaceMount`는 실제 프로젝트 디렉터리의 bind mount로 채운다.
Compose 템플릿은 `service`, `workspaceFolder`, 프로젝트 볼륨 마운트를 직접 지정해야 한다.
Compose 템플릿에도 기존 [사용자 Compose override](devcontainer-overrides.md)를 적용할 수 있다.

템플릿을 수정하거나 제거한 후 호스트에서 준비 명령을 실행하면 저장된 사본이 갱신된다.
실행 중인 컨테이너를 자동으로 교체하지 않는다. 기존 컨테이너에 적용하려면
`cxz project recreate WORKSPACE`를 실행한다. 재생성은 컨테이너 쓰기 계층을 교체하고
연결된 에디터를 끊으며 프로젝트 소스와 named volume은 유지한다.

이 기능을 지원하지 않는 Manager에 템플릿을 전달하면 CLI가 업그레이드 필요 오류를 표시한다.
