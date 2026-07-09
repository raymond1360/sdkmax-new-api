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
import { getLobeIcon } from '@/lib/lobe-icon'
import type { PricingModel } from '../types'

type ModelLogoRule = {
  pattern: RegExp
  icon: string
}

const MODEL_LOGO_RULES: ModelLogoRule[] = [
  { pattern: /\bclaude|anthropic/i, icon: 'Claude.Avatar' },
  { pattern: /\bgpt-|gpt\b|openai|o[1345](?:-|$)|chatgpt/i, icon: 'OpenAI.Avatar' },
  { pattern: /gemini|google/i, icon: 'Gemini.Avatar' },
  { pattern: /deepseek/i, icon: 'DeepSeek.Avatar' },
  { pattern: /\bgrok\b|xai|x-ai/i, icon: 'Grok.Avatar' },
  { pattern: /\bqwen\b|tongyi|alibaba/i, icon: 'Qwen.Avatar' },
  { pattern: /\bllama\b|meta/i, icon: 'Meta.Avatar' },
  { pattern: /mistral|mixtral|codestral/i, icon: 'Mistral.Avatar' },
  { pattern: /moonshot|\bkimi\b/i, icon: 'Moonshot.Avatar' },
  { pattern: /perplexity|sonar/i, icon: 'Perplexity.Avatar' },
  { pattern: /cohere|command-r/i, icon: 'Cohere.Avatar' },
  { pattern: /doubao|volc|volcengine/i, icon: 'Volcengine.Avatar' },
  { pattern: /ernie|wenxin|baidu/i, icon: 'Wenxin.Avatar' },
  { pattern: /zhipu|glm/i, icon: 'Zhipu.Avatar' },
  { pattern: /hunyuan|tencent/i, icon: 'Hunyuan.Avatar' },
  { pattern: /yi-|lingyi|01\.ai/i, icon: 'ZeroOne.Avatar' },
]

function normalizeIconName(iconName: string | undefined | null) {
  const trimmed = iconName?.trim()
  return trimmed || null
}

export function getModelLogoIconName(model: PricingModel) {
  const explicitModelIcon = normalizeIconName(model.icon)
  if (explicitModelIcon) return explicitModelIcon

  const haystack = [
    model.model_name,
    model.icon,
    model.vendor_name,
    model.vendor_icon,
    model.vendor_description,
    model.tags,
  ]
    .filter(Boolean)
    .join(' ')

  const matched = MODEL_LOGO_RULES.find((rule) => rule.pattern.test(haystack))
  if (matched) return matched.icon

  return normalizeIconName(model.vendor_icon)
}

export function renderModelLogo(model: PricingModel, size = 28) {
  return getLobeIcon(getModelLogoIconName(model), size)
}
