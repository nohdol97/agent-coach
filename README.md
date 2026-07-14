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

## 대상 플랫폼

Windows·Ubuntu 우선, macOS 병행. Go 단일 정적 바이너리 — 런타임 의존성 없음(비개발자 배포 전제).

## 상태

스캐폴딩 단계. MVP 스펙은 `docs/specs/`에 작성 후 구현한다.

## 라이선스

미정 (사내 배포 우선).
