// cc-usage band를 프롬프트 위(AbovePrompt)에 그린다 — 줄 구성은 userConfig의 layout·disabledWidgets를 따른다.
// 데이터 출처는 stdin이 아니라 mods API다 — $.session.usage()가 status line과 같은 수치를 준다.
import type { EngineInterface, On, PluginOptions } from 'claude-code'
import { type Language, resolveLanguage } from './i18n'
import { parseRemoteSlug } from './paths'
import { isActive, resolveSettings } from './settings'
import { buildLines, type Figures, type Part } from './widgets'

// 리셋 카운트다운과 브랜치를 세션이 쉬는 동안에도 갱신하는 주기
const TICK_MS = 30_000
// 갱신 경로가 잇따라 와도 같은 root에서 git·session.repo는 이 간격 안에 한 번만 돈다 (Go branchCacheTTL과 같음)
const LOOKUP_TTL_MS = 5_000

let usage: Figures | null = null
let model = ''
// cwd()는 셸 cd를 따라가 이름·브랜치가 흔들리므로 cd로 움직이지 않는 root()를 쓴다
let root = ''
let homes: string[] = []
let language: Language = 'en'

// root 하나의 브랜치·remote 조회 결과다. checkedAt은 조회를 시작한 $.clock.now() 시각이다.
type RootEntry = { branch: string; repoSlug: string | null; checkedAt: number; pending?: Promise<void> }

// 무엇을 조회할지와 root별 결과 — 플러그인을 다시 로드하면 새로 만든다
type Lookup = { withBranch: boolean; withRepo: boolean; cache: Map<string, RootEntry> }

export function register(on: On, options: PluginOptions) {
  // 옵션이 바뀌면 엔진이 플러그인을 다시 로드하므로 활성화 동안 설정은 그대로다
  const settings = resolveSettings(options)
  const lookup: Lookup = {
    withBranch: isActive(settings, 'projectInfo') || isActive(settings, 'projectName'),
    withRepo: isActive(settings, 'repoInfo'),
    cache: new Map(),
  }

  on('session.start', async ($, e, next) => {
    homes = await readHomes($)
    language = resolveLanguage(settings.language, await readLocales($))
    await refreshAll($, lookup)
    $.clock.every(TICK_MS, async () => {
      await refreshAll($, lookup)
      $.ui.invalidate('ui.render')
    })
    return next(e)
  })

  // 턴이 끝날 때마다, 그리고 rate limit이 1포인트 움직일 때 발생한다
  on('session.measure', async ($, e, next) => {
    usage = { context: e.context, rateLimits: e.rateLimits, cost: e.cost }
    $.ui.invalidate('ui.render')
    return next(e)
  })

  // /model 전환과 브랜치 이동은 measure에 잡히지 않으므로 턴 끝에서 다시 읽는다
  on('turn.complete', async ($, e, next) => {
    await refreshSession($, lookup)
    $.ui.invalidate('ui.render')
    return next(e)
  })

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    if (e.props.hasSurvey) return next(e)
    const { Box, Text } = $.ui.resolve(e)

    const entry = lookup.cache.get(root)
    const branch = entry?.branch ?? ''
    const repoSlug = entry?.repoSlug ?? null
    const now = await $.clock.now()
    // Box의 paddingLeft 1칸을 뺀 폭이 줄마다 쓸 수 있는 표시폭이다
    const budget = e.props.bodyColumns - 1
    const lines = buildLines({ usage, model, root, branch, repoSlug, homes, language, now }, settings, budget)
    if (lines.length === 0) return next(e)

    const textOf = (p: Part) =>
      Text({
        ...(p.color !== undefined && { color: p.color }),
        ...(p.dimColor !== undefined && { dimColor: p.dimColor }),
        ...(p.bold !== undefined && { bold: p.bold }),
        children: [p.text],
      })
    // mod의 표시폭 계산보다 터미널이 넓게 그리면(Ambiguous 문자를 2칸으로 그리는 터미널) 줄이 다음 행으로 넘어간다 — truncate-end가 막는다
    const children = lines.map((parts) => Text({ wrap: 'truncate-end', children: parts.map(textOf) }))
    // band는 spinner 바로 아래에 붙어 그려진다 — 위 한 줄을 비워 떼어 놓는다
    return Box({ flexDirection: 'column', marginTop: 1, paddingLeft: 1, children })
  })
}

async function refreshAll($: EngineInterface, lookup: Lookup) {
  usage = await $.session.usage()
  await refreshSession($, lookup)
}

async function refreshSession($: EngineInterface, lookup: Lookup) {
  model = await $.session.model()
  root = await $.session.root()
  await refreshRoot($, lookup)
}

// 진행 중인 조회가 있으면 그것을 함께 기다리고, TTL 안이면 캐시를 그대로 둔다.
// 캐시 확인과 항목 등록 사이에 await가 없어야 동시에 들어온 갱신이 조회를 늘리지 않는다.
async function refreshRoot($: EngineInterface, lookup: Lookup) {
  if (!lookup.withBranch && !lookup.withRepo) return
  const key = root
  const now = await $.clock.now()
  const cached = lookup.cache.get(key)
  if (cached?.pending) return cached.pending
  if (cached && now - cached.checkedAt < LOOKUP_TTL_MS) return

  const entry: RootEntry = { branch: cached?.branch ?? '', repoSlug: cached?.repoSlug ?? null, checkedAt: now }
  lookup.cache.set(key, entry)
  entry.pending = readRoot($, lookup, key, entry)
  await entry.pending
  entry.pending = undefined
}

// 실패는 빈 값으로 남겨 TTL이 지날 때까지 다시 시도하지 않는다
async function readRoot($: EngineInterface, lookup: Lookup, dir: string, entry: RootEntry) {
  if (lookup.withBranch) entry.branch = await readBranch($, dir)
  if (lookup.withRepo) entry.repoSlug = await readRepo($)
}

// claude plugin validate는 env 이름을 문자열 리터럴로만 받는다 — 변수로 넘기면 거부된다
async function readHomes($: EngineInterface): Promise<string[]> {
  const found: string[] = []
  for (const v of [await $.env.get('HOME'), await $.env.get('USERPROFILE')]) {
    if (v) found.push(v)
  }
  return found
}

// Go 판 detectLanguage와 같은 순서로 읽는다 — 이름은 readHomes처럼 리터럴이어야 한다
async function readLocales($: EngineInterface): Promise<(string | undefined)[]> {
  const values = [await $.env.get('LC_ALL'), await $.env.get('LC_MESSAGES'), await $.env.get('LANG')]
  return values.map((v) => v ?? undefined)
}

async function readBranch($: EngineInterface, dir: string): Promise<string> {
  try {
    const r = await $.process.run(['git', 'branch', '--show-current'], { cwd: dir, timeoutMs: 2000 })
    return r.exitCode === 0 ? r.stdout.trim() : ''
  } catch {
    // 실행이 거부되거나 시간을 넘겨도 band는 브랜치 괄호 없이 그린다
    return ''
  }
}

async function readRepo($: EngineInterface): Promise<string | null> {
  try {
    const repo = await $.session.repo()
    return repo?.remote ? parseRemoteSlug(repo.remote) : null
  } catch {
    // 조회가 거부되면 repoInfo만 생략한다
    return null
  }
}
