import { expect, test, type Page } from '@playwright/test'
import {
  browserInspectionScript,
  browserInspectionTextLimit,
  type BrowserInspectionText,
} from '../../desktop/src/browserInspection'

async function inspectPage(page: Page) {
  return page.evaluate(
    (script) => (0, eval)(script) as BrowserInspectionText,
    browserInspectionScript,
  )
}

test('browser inspection includes rendered text and excludes sensitive or hidden content', async ({
  page,
}) => {
  await page.setContent(`
    <!doctype html>
    <main>
      <h1>Visible heading</h1>
      <button>Continue</button>
      <label>Account <input value="secret input"></label>
      <textarea>secret textarea</textarea>
      <select><option>secret option</option></select>
      <div contenteditable="true">secret editable</div>
      <div hidden>secret hidden</div>
      <div inert>secret inert</div>
      <div aria-hidden="true">secret aria</div>
      <div style="display:none">secret display</div>
      <div style="visibility:hidden">secret visibility</div>
      <div style="opacity:0"><span>secret transparent ancestor</span></div>
      <script>window.secretScript = 'secret script text'</script>
    </main>
  `)

  const result = await inspectPage(page)
  expect(result).toEqual({
    visibleText: 'Visible heading\nContinue\nAccount',
    truncated: false,
  })
  expect(result.visibleText).not.toContain('secret')
})

test('browser inspection bounds visible text and marks truncation', async ({ page }) => {
  await page.setContent(`<main>${'x'.repeat(browserInspectionTextLimit + 100)}</main>`)

  const result = await inspectPage(page)
  expect(result.visibleText).toHaveLength(browserInspectionTextLimit)
  expect(result.truncated).toBe(true)
})
