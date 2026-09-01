import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'

describe('API keys desktop table overflow contract', () => {
  test('keeps pagination outside a horizontally scrollable table with stable width', () => {
    const tablePageSource = readFileSync(
      new URL(
        '../../../components/data-table/data-table-page.tsx',
        import.meta.url
      ),
      'utf8'
    )
    const apiKeysTableSource = readFileSync(
      new URL('./api-keys-table.tsx', import.meta.url),
      'utf8'
    )

    assert.match(tablePageSource, /tableElementClassName\?: string/)
    assert.match(
      tablePageSource,
      /<Table className=\{props\.tableElementClassName\}>/
    )
    assert.match(apiKeysTableSource, /className='w-full min-w-0'/)
    assert.match(
      apiKeysTableSource,
      /tableClassName='w-full max-w-full min-w-0'/
    )
    assert.match(apiKeysTableSource, /tableElementClassName='min-w-\[1280px\]'/)
  })
})
