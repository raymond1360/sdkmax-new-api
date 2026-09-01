import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getCollectionModelNames,
  SDKMAX_MODEL_COLLECTIONS,
} from './sdkmax-model-collections.ts'

describe('SDKMAX model collections', () => {
  test('returns every matching scene model instead of truncating at 16', () => {
    const codingModels = Array.from(
      { length: 24 },
      (_, index) => `vendor/coder-model-${index + 1}`
    )
    const otherModels = ['vendor/general-chat', 'vendor/text-model']

    const selectedModels = getCollectionModelNames(
      [...codingModels, ...otherModels],
      'coding'
    )

    assert.equal(selectedModels.length, codingModels.length)
    assert.deepEqual(selectedModels, codingModels)
  })

  test('uses the same collection result for card counts and model_limits', () => {
    const models = [
      'openai/gpt-5',
      'anthropic/claude-sonnet-4',
      'deepseek/deepseek-coder',
      'hidden/coder-customer-visible-false',
    ]
    const visibleModels = models.filter(
      (modelName) => modelName !== 'hidden/coder-customer-visible-false'
    )

    const cardCount = getCollectionModelNames(visibleModels, 'coding').length
    const modelLimits = getCollectionModelNames(visibleModels, 'coding')

    assert.equal(cardCount, modelLimits.length)
    assert.equal(
      modelLimits.includes('hidden/coder-customer-visible-false'),
      false
    )
  })

  test('does not include developer mode in ordinary API key scenes', () => {
    assert.deepEqual(
      SDKMAX_MODEL_COLLECTIONS.map((collection) => collection.id),
      [
        'recommended',
        'coding',
        'writing',
        'research',
        'enterprise',
        'value',
        'chinese',
      ]
    )
  })
})
