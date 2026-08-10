/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export type SdkmaxModelCollectionId =
  | 'recommended'
  | 'coding'
  | 'writing'
  | 'research'
  | 'enterprise'
  | 'value'
  | 'chinese'
  | 'developer'

export type SdkmaxModelCollection = {
  id: SdkmaxModelCollectionId
  title: string
  description: string
  intent: string
  patterns: RegExp[]
  tags: string[]
}

export type OfficialModelPrice = {
  input: number
  output: number
  label: string
  source: string
}

export const SDKMAX_MODEL_COLLECTIONS: SdkmaxModelCollection[] = [
  {
    id: 'recommended',
    title: 'Smart picks',
    description: 'Balanced models for most chat, tool use, and agent tasks.',
    intent: 'Start here when you are not sure which model to choose.',
    patterns: [/gpt-5/i, /claude.*sonnet/i, /gemini.*pro/i, /deepseek/i],
    tags: ['balanced', 'agent', 'general'],
  },
  {
    id: 'coding',
    title: 'Coding assistant',
    description: 'Models that work well for code generation and debugging.',
    intent: 'Build, review, refactor, and explain code.',
    patterns: [/claude.*sonnet/i, /gpt-5/i, /coder/i, /code/i, /deepseek/i],
    tags: ['code', 'debug', 'review'],
  },
  {
    id: 'writing',
    title: 'Content creation',
    description: 'Strong language models for drafts, rewriting, and ideation.',
    intent: 'Write articles, emails, summaries, and creative content.',
    patterns: [/gpt-5/i, /gpt-4/i, /claude/i, /gemini/i, /qwen.*plus/i],
    tags: ['writing', 'summary', 'creative'],
  },
  {
    id: 'research',
    title: 'Study and research',
    description: 'Long-context and reasoning models for deeper analysis.',
    intent: 'Read long material, compare evidence, and reason step by step.',
    patterns: [/reason/i, /thinking/i, /opus/i, /sonnet/i, /pro/i, /long/i],
    tags: ['reasoning', 'long-context', 'analysis'],
  },
  {
    id: 'enterprise',
    title: 'Enterprise work',
    description: 'Reliable models for production workflows and team apps.',
    intent: 'Use in customer support, internal tools, and business workflows.',
    patterns: [/gpt-5/i, /claude/i, /gemini.*pro/i, /command/i],
    tags: ['stable', 'business', 'workflow'],
  },
  {
    id: 'value',
    title: 'Value assistant',
    description: 'Cost-effective models for high-volume daily tasks.',
    intent: 'Keep routine usage affordable without losing useful quality.',
    patterns: [/mini/i, /flash/i, /lite/i, /turbo/i, /deepseek/i, /qwen/i],
    tags: ['budget', 'fast', 'volume'],
  },
  {
    id: 'chinese',
    title: 'Chinese assistant',
    description: 'Models with strong Chinese and local-language support.',
    intent:
      'Chinese writing, translation, customer service, and knowledge work.',
    patterns: [/qwen/i, /deepseek/i, /glm/i, /kimi/i, /moonshot/i, /hunyuan/i],
    tags: ['chinese', 'translation', 'local'],
  },
  {
    id: 'developer',
    title: 'Developer mode',
    description: 'Flexible model access for engineers who want manual control.',
    intent: 'Choose exactly which models a key can call.',
    patterns: [/./],
    tags: ['advanced', 'manual', 'api'],
  },
]

const OFFICIAL_PRICE_RULES: Array<{
  pattern: RegExp
  price: OfficialModelPrice
}> = [
  {
    pattern: /gpt-5-mini/i,
    price: {
      input: 0.25,
      output: 2,
      label: 'OpenAI official',
      source: 'https://developers.openai.com/api/docs/pricing',
    },
  },
  {
    pattern: /gpt-5(?:$|[^a-z])/i,
    price: {
      input: 1.25,
      output: 10,
      label: 'OpenAI official',
      source: 'https://developers.openai.com/api/docs/pricing',
    },
  },
  {
    pattern: /gpt-5-nano/i,
    price: {
      input: 0.05,
      output: 0.4,
      label: 'OpenAI official',
      source: 'https://developers.openai.com/api/docs/pricing',
    },
  },
  {
    pattern: /claude.*opus/i,
    price: {
      input: 5,
      output: 25,
      label: 'Anthropic official',
      source: 'https://platform.claude.com/docs/en/about-claude/pricing',
    },
  },
  {
    pattern: /claude.*sonnet/i,
    price: {
      input: 3,
      output: 15,
      label: 'Anthropic official',
      source: 'https://platform.claude.com/docs/en/about-claude/pricing',
    },
  },
  {
    pattern: /claude.*haiku/i,
    price: {
      input: 0.8,
      output: 4,
      label: 'Anthropic official',
      source: 'https://platform.claude.com/docs/en/about-claude/pricing',
    },
  },
  {
    pattern: /gemini.*pro/i,
    price: {
      input: 2.7,
      output: 16.2,
      label: 'Google official',
      source: 'https://ai.google.dev/gemini-api/docs/pricing',
    },
  },
  {
    pattern: /gemini.*flash/i,
    price: {
      input: 0.15,
      output: 1.25,
      label: 'Google official',
      source: 'https://ai.google.dev/gemini-api/docs/pricing',
    },
  },
  {
    pattern: /deepseek.*(?:reasoner|r\d|pro)/i,
    price: {
      input: 0.435,
      output: 0.87,
      label: 'DeepSeek official',
      source: 'https://api-docs.deepseek.com/quick_start/pricing',
    },
  },
  {
    pattern: /deepseek/i,
    price: {
      input: 0.14,
      output: 0.28,
      label: 'DeepSeek official',
      source: 'https://api-docs.deepseek.com/quick_start/pricing',
    },
  },
  {
    pattern: /qwen.*(?:max|coder)/i,
    price: {
      input: 1.25,
      output: 3.75,
      label: 'Alibaba Cloud official',
      source: 'https://www.alibabacloud.com/help/en/model-studio/models',
    },
  },
  {
    pattern: /qwen.*plus/i,
    price: {
      input: 0.4,
      output: 2.4,
      label: 'Alibaba Cloud official',
      source: 'https://www.alibabacloud.com/help/en/model-studio/models',
    },
  },
  {
    pattern: /qwen/i,
    price: {
      input: 0.1,
      output: 0.4,
      label: 'Alibaba Cloud official',
      source: 'https://www.alibabacloud.com/help/en/model-studio/models',
    },
  },
]

export function getCollectionById(id: string | null | undefined) {
  return SDKMAX_MODEL_COLLECTIONS.find((collection) => collection.id === id)
}

export function modelMatchesCollection(
  modelName: string,
  collection: SdkmaxModelCollection
) {
  return collection.patterns.some((pattern) => pattern.test(modelName))
}

export function getModelCollections(modelName: string) {
  return SDKMAX_MODEL_COLLECTIONS.filter(
    (collection) =>
      collection.id !== 'developer' &&
      modelMatchesCollection(modelName, collection)
  )
}

export function getOfficialModelPrice(modelName: string) {
  return OFFICIAL_PRICE_RULES.find((rule) => rule.pattern.test(modelName))
    ?.price
}

export function getCollectionModelNames<T extends { model_name?: string }>(
  models: Array<T | string>,
  collectionId: SdkmaxModelCollectionId,
  limit = 12
) {
  const collection = getCollectionById(collectionId)
  if (!collection) return []
  return models
    .map((model) => (typeof model === 'string' ? model : model.model_name))
    .filter((modelName): modelName is string =>
      modelName ? modelMatchesCollection(modelName, collection) : false
    )
    .slice(0, limit)
}
