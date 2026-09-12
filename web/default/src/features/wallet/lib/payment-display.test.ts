import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { getPaidAmountDisplay } from './payment-display.ts'

describe('wallet payment display', () => {
  test('successful Stripe orders show the actual paid minor amount', () => {
    assert.deepEqual(
      getPaidAmountDisplay({
        status: 'success',
        currency: 'USD',
        money: 10,
        paid_amount_minor: 1000,
      }),
      {
        kind: 'paid_minor',
        amountMinor: 1000,
        currency: 'USD',
      }
    )
  })

  test('failed Stripe orders show unpaid instead of expected amount', () => {
    assert.deepEqual(
      getPaidAmountDisplay({
        status: 'failed',
        currency: 'USD',
        money: 5,
        paid_amount_minor: 0,
      }),
      {
        kind: 'unpaid',
      }
    )
  })

  test('pending Stripe orders show unpaid instead of expected amount', () => {
    assert.deepEqual(
      getPaidAmountDisplay({
        status: 'pending',
        currency: 'USD',
        money: 5,
        paid_amount_minor: 0,
      }),
      {
        kind: 'unpaid',
      }
    )
  })

  test('non-Stripe records keep the legacy money display path', () => {
    assert.deepEqual(
      getPaidAmountDisplay({
        status: 'success',
        money: 5,
      }),
      {
        kind: 'legacy_money',
        money: 5,
      }
    )
  })
})
