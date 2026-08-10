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
import { memo } from 'react'
import { ChevronRight, Copy, KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { formatCurrencyFromUSD } from '@/lib/currency'
import {
  getModelCollections,
  getOfficialModelPrice,
} from '@/lib/sdkmax-model-collections'
import { cn } from '@/lib/utils'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { StatusBadge } from '@/components/status-badge'
import { DEFAULT_TOKEN_UNIT } from '../constants'
import { parseTags } from '../lib/filters'
import { isTokenBasedModel } from '../lib/model-helpers'
import { renderModelLogo } from '../lib/model-logo'
import { getSdkmaxTokenPriceSnapshot, stripTrailingZeros } from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'
import { ModelPerfBadge, type ModelPerfBadgeData } from './model-perf-badge'

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  perf?: ModelPerfBadgeData
  onUse?: (modelName: string) => void
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const isTokenBased = isTokenBasedModel(props.model)
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const tags = parseTags(props.model.tags)
  const groups = props.model.enable_groups || []
  const endpoints = props.model.supported_endpoint_types || []
  const modelLogo = renderModelLogo(props.model, 28)
  const isDynamicPricing =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)

  const primaryGroup = groups[0]
  const bottomTags = [...endpoints.slice(0, 2), ...tags.slice(0, 2)]
  const collections = getModelCollections(props.model.model_name)
  const officialPrice = getOfficialModelPrice(props.model.model_name)
  const sdkmaxPrice = getSdkmaxTokenPriceSnapshot(props.model)
  const canCompare = Boolean(officialPrice && sdkmaxPrice)
  const officialTotal = officialPrice
    ? officialPrice.input + officialPrice.output
    : 0
  const sdkmaxTotal = sdkmaxPrice ? sdkmaxPrice.input + sdkmaxPrice.output : 0
  const officialLower = canCompare && officialTotal < sdkmaxTotal
  const priceDeltaPercent =
    canCompare && officialTotal > 0 && sdkmaxTotal > 0
      ? Math.max(
          0,
          Math.round(
            (officialLower
              ? sdkmaxTotal / officialTotal - 1
              : 1 - sdkmaxTotal / officialTotal) * 100
          )
        )
      : 0
  const hiddenCount =
    Math.max(groups.length - 1, 0) +
    Math.max(endpoints.length - 2, 0) +
    Math.max(tags.length - 2, 0)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  const handleUse = (e: React.MouseEvent) => {
    e.stopPropagation()
    props.onUse?.(props.model.model_name || '')
  }

  return (
    <div
      className={cn(
        'group relative flex flex-col rounded-xl border p-3 transition-colors sm:p-5',
        'hover:bg-muted/20'
      )}
    >
      {/* Header: icon + name + actions */}
      <div className='flex items-start justify-between gap-2.5 sm:gap-3'>
        <div className='flex min-w-0 items-start gap-2.5 sm:gap-3'>
          <div className='bg-background flex size-9 shrink-0 items-center justify-center rounded-lg border shadow-sm sm:size-10 sm:rounded-xl'>
            {modelLogo}
          </div>
          <div className='min-w-0'>
            <h3 className='text-foreground truncate font-mono text-[15px] leading-tight font-bold'>
              {props.model.model_name}
            </h3>
            <div className='mt-0.5 flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-xs sm:mt-1 sm:gap-x-3'>
              {props.model.vendor_name && (
                <span className='text-muted-foreground truncate'>
                  {props.model.vendor_name}
                </span>
              )}
              {props.model.context_length && (
                <span className='text-muted-foreground/70 whitespace-nowrap'>
                  Context: {props.model.context_length}
                </span>
              )}
            </div>
          </div>
        </div>

        <div className='flex shrink-0 items-center gap-1.5'>
          <button
            type='button'
            onClick={props.onClick}
            className='text-muted-foreground hover:text-foreground hover:bg-muted inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors sm:px-2.5 sm:py-1.5'
          >
            {t('Details')}
            <ChevronRight className='size-3.5' />
          </button>
          <button
            type='button'
            onClick={handleCopy}
            className='text-muted-foreground hover:text-foreground hover:bg-muted rounded-md border p-1.5 transition-colors'
            title={t('Copy')}
          >
            <Copy className='size-3.5' />
          </button>
        </div>
      </div>

      {/* Description */}
      <p className='text-muted-foreground mt-2 line-clamp-1 flex-1 text-[13px] leading-relaxed sm:mt-4 sm:line-clamp-2 sm:min-h-[2.5rem]'>
        {props.model.description || t('No description available.')}
      </p>

      <div className='mt-3 flex flex-wrap gap-1.5'>
        {collections.slice(0, 2).map((collection) => (
          <StatusBadge
            key={collection.id}
            label={t(collection.title)}
            variant='info'
            copyable={false}
            size='sm'
          />
        ))}
      </div>

      <div className='bg-muted/20 mt-3 rounded-lg border p-3'>
        {canCompare && officialPrice && sdkmaxPrice ? (
          <div className='space-y-2'>
            <div className='flex items-center justify-between gap-2'>
              <span className='text-muted-foreground text-xs'>
                {t('Official vs SDKMAX')}
              </span>
              <span
                className={cn(
                  'rounded-md px-2 py-0.5 text-xs font-semibold',
                  officialLower
                    ? 'bg-amber-500/10 text-amber-700 dark:text-amber-300'
                    : 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                )}
              >
                {officialLower
                  ? t('Official lower {{percent}}%', {
                      percent: priceDeltaPercent,
                    })
                  : t('Save {{percent}}%', { percent: priceDeltaPercent })}
              </span>
            </div>
            <div className='grid grid-cols-2 gap-2 text-xs'>
              <div>
                <div className='text-muted-foreground/70'>{t('Official')}</div>
                <div className='font-mono tabular-nums'>
                  {stripTrailingZeros(
                    formatCurrencyFromUSD(officialPrice.input)
                  )}{' '}
                  /{' '}
                  {stripTrailingZeros(
                    formatCurrencyFromUSD(officialPrice.output)
                  )}
                </div>
              </div>
              <div>
                <div className='text-muted-foreground/70'>SDKMAX</div>
                <div className='font-mono tabular-nums'>
                  {stripTrailingZeros(formatCurrencyFromUSD(sdkmaxPrice.input))}{' '}
                  /{' '}
                  {stripTrailingZeros(
                    formatCurrencyFromUSD(sdkmaxPrice.output)
                  )}
                </div>
              </div>
            </div>
            <div className='text-muted-foreground/50 text-[10px]'>
              {t('/ 1M input and output tokens')}
            </div>
          </div>
        ) : sdkmaxPrice ? (
          <div className='space-y-1'>
            <div className='text-muted-foreground text-xs'>
              {t('SDKMAX price')}
            </div>
            <div className='font-mono text-sm tabular-nums'>
              {stripTrailingZeros(formatCurrencyFromUSD(sdkmaxPrice.input))} /{' '}
              {stripTrailingZeros(formatCurrencyFromUSD(sdkmaxPrice.output))}
            </div>
            <div className='text-muted-foreground/50 text-[10px]'>
              {t('/ 1M input and output tokens')}
            </div>
          </div>
        ) : (
          <div className='text-muted-foreground text-xs'>
            {t('See details for pricing')}
          </div>
        )}
      </div>

      {/* Footer: left metadata and right performance summary share row alignment */}
      <div className='mt-2 grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-2 gap-y-1 sm:mt-4'>
        <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1'>
          {primaryGroup && (
            <span className='text-muted-foreground text-xs font-medium'>
              {primaryGroup} {t('Groups')}
            </span>
          )}
          <span className='text-muted-foreground text-xs font-medium'>
            {isTokenBased ? t('Token-based') : t('Per Request')}
          </span>
          {isDynamicPricing && (
            <StatusBadge
              label={t('Dynamic Pricing')}
              variant='warning'
              copyable={false}
              size='sm'
            />
          )}
        </div>
        <ModelPerfBadge perf={props.perf} className='row-span-2 self-start' />

        <div className='flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5 sm:gap-x-3 sm:gap-y-1'>
          {bottomTags.map((item) => (
            <span key={item} className='text-muted-foreground/70 text-xs'>
              {item}
            </span>
          ))}
          <span className='text-muted-foreground/50 text-xs'>
            {tokenUnitLabel}
          </span>
          {hiddenCount > 0 && (
            <span className='text-muted-foreground/40 text-xs'>
              +{hiddenCount}
            </span>
          )}
        </div>
      </div>

      <button
        type='button'
        onClick={handleUse}
        className='bg-primary text-primary-foreground hover:bg-primary/90 mt-4 inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium transition-colors'
      >
        <KeyRound className='size-4' />
        {t('Use now')}
      </button>
    </div>
  )
})
