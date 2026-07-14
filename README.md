# agent-coach

> AI 에이전트(Claude Code·Codex) 사용을 매일 분석해 토큰 비효율을 찾아내고, 개선 지침을 사용자의 전역 에이전트 설정(`~/.claude`, `~/.codex`)에 자동 기입하는 코칭 서비스.
> Daily agent-usage coaching: analyzes your Claude Code / Codex sessions via [agentsview](https://github.com/kenn-io/agentsview) and writes token-efficiency guidance into your global agent config.

## 왜 만드는가

측정 도구(agentsview)는 이미 있다. 없는 것은 **피드백 루프**다 — 측정 결과를 읽고, 비효율 패턴을 찾고, 다음 세션의 에이전트 행동을 바꾸는 지침으로 되돌리는 일은 사람이 매일 하지 않는다. agent-coach는 그 루프를 자동화해 사내 에이전트 사용자의 토큰 낭비를 구조적으로 줄인다.

## 동작 개요

```
설치 시 1회  : agentcoach install
              ├─ agentsview CLI 설치·데몬 기동 보장
              ├─ OS 스케줄러 등록 (Windows 작업 스케줄러 / systemd user timer / launchd)
              └─ ~/.agentcoach/ 초기화 (state.json, config.toml)

매일 자동    : agentcoach analyze
              ① Collector — agentsview 로컬 DB에서 지난 측정(워터마크) 이후 세션만 조회
              ② Analyzer  — 규칙 기반 지표 → 비효율 발견 (LLM 불사용, 결정적)
              ③ Advisor   — 발견 → 개선 지침 (상위 3개, 15줄 상한)
              ④ Writer    — ~/.claude/CLAUDE.md · ~/.codex/AGENTS.md 의 관리 블록 갱신
                            + 전체 리포트 ~/.agentcoach/reports/YYYY-MM-DD.md
              ⑤ 데스크톱 알림 — "지난 측정 대비 토큰 ±N% · 새 코칭 M건"
```

- **hook 미사용**: 트리거는 OS 스케줄러다. Claude Code·Codex 어느 쪽 사용자든 동일하게 동작한다.
- **관리 블록**: 사용자 파일에는 `<!-- agentcoach:begin -->` ~ `<!-- agentcoach:end -->` 센티널 사이만 쓴다. 쓰기 전 백업, 마커 훼손 시 쓰기 포기(fail-open). 블록은 15줄 상한 — 전역 지침은 모든 세션에 로드되므로 긴 코칭은 그 자체가 토큰 낭비다.
- **데이터는 머신 밖으로 나가지 않는다**: 세션 로그에는 회사 데이터·시크릿이 섞일 수 있다. 분석·리포트 전부 로컬.

## 설치 (비개발자 기준 2단계)

1. [Releases](https://github.com/nohdol97/agent-coach/releases)에서 OS에 맞는 파일을 받아 압축을 풀고, `agentcoach`(Windows는 `agentcoach.exe`)를 적당한 위치에 둔다.
2. 터미널(Windows는 PowerShell)에서 한 번 실행한다:

```
agentcoach install
```

이것으로 끝이다 — agentsview가 없으면 자동 설치를 시도하고, OS 스케줄러에 매일 09:30 실행을 등록하고, 첫 분석까지 수행한다. 이후에는 매일 자동으로 분석·기입·알림이 이뤄진다.

```
agentcoach install --time 08:00     # 실행 시각 변경
agentcoach analyze --force          # 지금 즉시 재분석
agentcoach analyze --dry-run        # 파일을 건드리지 않고 미리보기
agentcoach report                   # 최신 리포트 보기
agentcoach uninstall                # 스케줄 해제 + 관리 블록 제거 (데이터 보존)
agentcoach uninstall --purge        # 데이터까지 삭제
```

## 대상 플랫폼

Windows·Ubuntu 우선(모든 커밋을 두 OS CI에서 검증), macOS 병행. Go 단일 정적 바이너리 — 런타임 의존성 없음(비개발자 배포 전제, `go.mod`에 외부 모듈 0).

## 분석 규칙 (전부 결정적 — LLM 불사용)

| 규칙 | 신호 | 코칭 |
|---|---|---|
| trend | 지난 측정 대비 토큰·비용 증감 | 리포트 헤더·알림에 표시 |
| heavy-context | peak context 임계(기본 150k) 초과 세션 | 부분 읽기·세션 분리 |
| compaction | 컨텍스트 압축(특히 작업 중) 발생 | 작업을 미리 나누기 |
| prompt-hygiene | 짧은/중복 프롬프트, 검증 기준 부재, 도구 루프 | 첫 프롬프트에 요구사항·완료 기준 |
| rework | 도구 실패·재시도·수정 반복·낙제 등급 | 원인 확인 후 진행 |
| model-mix | 최상위 모델 비용 점유 + 소형 세션 | 소형 작업은 하위 모델로 |

임계값은 `~/.agentcoach/config.json`에서 조정한다.

## 개발

```
go test ./...   # 외부 의존 없음 — 표준 라이브러리만
go vet ./...
```

스펙: [docs/specs/2026-07-14-mvp.md](docs/specs/2026-07-14-mvp.md) (요구사항 R1~R9, 완료 기준 C1~C9).

## 라이선스

미정 (사내 배포 우선).
