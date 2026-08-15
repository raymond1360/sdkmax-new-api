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
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  SDKMAX_MODEL_COLLECTIONS,
  getOfficialModelPrice,
  modelMatchesCollection,
  type SdkmaxModelCollectionId,
} from '@/lib/sdkmax-model-collections'
import { cn } from '@/lib/utils'
import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import {
  LoadingSkeleton,
  EmptyState,
  SearchBar,
  PricingTable,
  PricingSidebar,
  PricingToolbar,
  ModelCardGrid,
  ModelDetailsDrawer,
} from './components'
import { EXCLUDED_GROUPS, VIEW_MODES } from './constants'
import { useFilters } from './hooks/use-filters'
import { usePricingData } from './hooks/use-pricing-data'
import { getSdkmaxTokenPriceSnapshot } from './lib/price'
import type { PricingModel } from './types'

function hasSdkmaxLowerPrice(model: PricingModel) {
  const officialPrice = getOfficialModelPrice(model.model_name)
  const sdkmaxPrice = getSdkmaxTokenPriceSnapshot(model)
  if (!officialPrice || !sdkmaxPrice) return false
  return (
    sdkmaxPrice.input + sdkmaxPrice.output <
    officialPrice.input + officialPrice.output
  )
}

export function Pricing() {
  const { t } = useTranslation()
  const [selectedModelName, setSelectedModelName] = useState<string | null>(
    null
  )
  const [selectedCollection, setSelectedCollection] = useState<
    SdkmaxModelCollectionId | 'all'
  >('recommended')

  const {
    models,
    vendors,
    groupRatio,
    usableGroup,
    endpointMap,
    autoGroups,
    isLoading,
    priceRate,
    usdExchangeRate,
  } = usePricingData()

  const {
    searchInput,
    sortBy,
    vendorFilter,
    groupFilter,
    quotaTypeFilter,
    endpointTypeFilter,
    tagFilter,
    tokenUnit,
    viewMode,
    showRechargePrice,
    setSearchInput,
    setSortBy,
    setVendorFilter,
    setGroupFilter,
    setQuotaTypeFilter,
    setEndpointTypeFilter,
    setTagFilter,
    setTokenUnit,
    setViewMode,
    setShowRechargePrice,
    filteredModels,
    hasActiveFilters,
    activeFilterCount,
    availableTags,
    clearFilters,
    clearSearch,
  } = useFilters(models || [])

  const handleModelClick = useCallback((modelName: string) => {
    setSelectedModelName(modelName)
  }, [])

  const selectedModel = useMemo(
    () =>
      selectedModelName
        ? (models || []).find(
            (model) => model.model_name === selectedModelName
          ) || null
        : null,
    [models, selectedModelName]
  )

  const collectionCounts = useMemo(() => {
    const counts = new Map<SdkmaxModelCollectionId, number>()
    for (const collection of SDKMAX_MODEL_COLLECTIONS) {
      counts.set(
        collection.id,
        (models || []).filter((model) =>
          modelMatchesCollection(model.model_name || '', collection)
        ).length
      )
    }
    return counts
  }, [models])

  const collectionFilteredModels = useMemo(() => {
    let result = filteredModels
    if (selectedCollection === 'all') {
      result = filteredModels
    } else {
      const collection = SDKMAX_MODEL_COLLECTIONS.find(
        (item) => item.id === selectedCollection
      )
      if (collection) {
        result = filteredModels.filter((model) =>
          modelMatchesCollection(model.model_name || '', collection)
        )
      }
    }

    return [...result].sort(
      (a, b) => Number(hasSdkmaxLowerPrice(b)) - Number(hasSdkmaxLowerPrice(a))
    )
  }, [filteredModels, selectedCollection])

  const availableGroups = useMemo(
    () =>
      Object.keys(usableGroup || {}).filter(
        (g) => !EXCLUDED_GROUPS.includes(g)
      ),
    [usableGroup]
  )

  const handleClearAll = useCallback(() => {
    clearFilters()
    clearSearch()
    setSelectedCollection('all')
  }, [clearFilters, clearSearch])

  const handleUseModel = useCallback((modelName: string) => {
    if (typeof window === 'undefined') return
    window.sessionStorage.setItem(
      'sdkmax.apiKeyPreset',
      JSON.stringify({ source: 'pricing', models: [modelName] })
    )
    window.location.assign('/keys')
  }, [])

  const renderPricingContent = () => {
    if (collectionFilteredModels.length === 0) {
      return (
        <EmptyState
          searchQuery={searchInput}
          hasActiveFilters={hasActiveFilters}
          onClearFilters={handleClearAll}
        />
      )
    }

    if (viewMode === VIEW_MODES.CARD) {
      return (
        <ModelCardGrid
          models={collectionFilteredModels}
          onModelClick={handleModelClick}
          priceRate={priceRate}
          usdExchangeRate={usdExchangeRate}
          tokenUnit={tokenUnit}
          showRechargePrice={showRechargePrice}
          onUseModel={handleUseModel}
        />
      )
    }

    return (
      <PricingTable
        models={collectionFilteredModels}
        priceRate={priceRate}
        usdExchangeRate={usdExchangeRate}
        tokenUnit={tokenUnit}
        showRechargePrice={showRechargePrice}
        onModelClick={handleModelClick}
      />
    )
  }

  if (isLoading) {
    return (
      <PublicLayout showMainContainer={false}>
        <div className='mx-auto w-full max-w-[1800px] px-3 pt-16 pb-8 sm:px-6 sm:pt-20 sm:pb-10 xl:px-8'>
          <LoadingSkeleton viewMode={viewMode} />
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout showMainContainer={false}>
      <div className='relative'>
        <div
          aria-hidden
          className='pointer-events-none absolute inset-x-0 top-0 h-[600px] opacity-20 dark:opacity-[0.10]'
          style={{
            background: [
              'radial-gradient(ellipse 60% 50% at 20% 20%, oklch(0.72 0.18 250 / 80%) 0%, transparent 70%)',
              'radial-gradient(ellipse 50% 40% at 80% 15%, oklch(0.65 0.15 200 / 60%) 0%, transparent 70%)',
              'radial-gradient(ellipse 40% 35% at 50% 70%, oklch(0.70 0.12 280 / 40%) 0%, transparent 70%)',
            ].join(', '),
            maskImage:
              'linear-gradient(to bottom, black 40%, transparent 100%)',
            WebkitMaskImage:
              'linear-gradient(to bottom, black 40%, transparent 100%)',
          }}
        />
        <PageTransition className='relative mx-auto w-full max-w-[1800px] px-3 pt-16 pb-8 sm:px-6 sm:pt-20 sm:pb-10 xl:px-8'>
          <header className='mx-auto mb-5 max-w-3xl pt-5 text-center sm:mb-10 sm:pt-10'>
            <p className='text-muted-foreground mb-3 text-xs font-medium tracking-widest uppercase'>
              {t('AI model selection center')}
            </p>
            <h1 className='text-[clamp(2rem,5.5vw,3.5rem)] leading-[1.15] font-bold tracking-tight'>
              SDKMAX
            </h1>
            <p className='text-muted-foreground/80 mt-3 text-sm sm:mt-4 sm:text-base'>
              {t('This site currently has {{count}} models enabled', {
                count: models?.length || 0,
              })}
            </p>
            <p className='text-muted-foreground/60 mx-auto mt-2 max-w-2xl text-xs leading-relaxed sm:text-sm'>
              {t(
                'Discover curated AI models, compare pricing and capabilities, and choose the right model for every scenario.'
              )}
            </p>
            <SearchBar
              value={searchInput}
              onChange={setSearchInput}
              onClear={clearSearch}
              placeholder={t(
                'Search model name, provider, endpoint, or tag...'
              )}
              className='mx-auto mt-4 max-w-2xl sm:mt-6'
            />
          </header>

          <section className='mb-5 space-y-3 sm:mb-6'>
            <div className='flex items-center justify-between gap-3'>
              <div>
                <h2 className='text-base font-semibold'>
                  {t('Choose by scenario')}
                </h2>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    'Model collections are recommendation presets and do not change billing groups.'
                  )}
                </p>
              </div>
              <button
                type='button'
                onClick={() => setSelectedCollection('all')}
                className={cn(
                  'hidden h-8 rounded-md border px-3 text-xs font-medium transition-colors sm:inline-flex sm:items-center',
                  selectedCollection === 'all'
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted'
                )}
              >
                {t('All models')}
              </button>
            </div>
            <div className='grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4'>
              {SDKMAX_MODEL_COLLECTIONS.map((collection) => {
                const active = selectedCollection === collection.id
                return (
                  <button
                    key={collection.id}
                    type='button'
                    onClick={() => setSelectedCollection(collection.id)}
                    className={cn(
                      'rounded-lg border p-3 text-left transition-colors',
                      active
                        ? 'border-primary bg-primary/5'
                        : 'hover:bg-muted/40'
                    )}
                  >
                    <div className='flex items-center justify-between gap-2'>
                      <span className='font-medium'>{t(collection.title)}</span>
                      <span className='text-muted-foreground text-xs tabular-nums'>
                        {collectionCounts.get(collection.id) || 0}
                      </span>
                    </div>
                    <p className='text-muted-foreground mt-1 line-clamp-2 text-xs'>
                      {t(collection.intent)}
                    </p>
                  </button>
                )
              })}
            </div>
          </section>

          <div className='grid gap-4 xl:grid-cols-[330px_minmax(0,1fr)]'>
            <PricingSidebar
              quotaTypeFilter={quotaTypeFilter}
              endpointTypeFilter={endpointTypeFilter}
              vendorFilter={vendorFilter}
              groupFilter={groupFilter}
              tagFilter={tagFilter}
              onQuotaTypeChange={setQuotaTypeFilter}
              onEndpointTypeChange={setEndpointTypeFilter}
              onVendorChange={setVendorFilter}
              onGroupChange={setGroupFilter}
              onTagChange={setTagFilter}
              vendors={vendors || []}
              groups={availableGroups}
              groupRatios={groupRatio}
              tags={availableTags}
              models={models || []}
              hasActiveFilters={hasActiveFilters}
              onClearFilters={clearFilters}
              className='hover-scrollbar sticky top-4 hidden max-h-[calc(100dvh-2rem)] self-start overflow-y-auto xl:block'
            />

            <main className='min-w-0 space-y-4'>
              <PricingToolbar
                filteredCount={collectionFilteredModels.length}
                totalCount={models?.length}
                sortBy={sortBy}
                onSortChange={setSortBy}
                tokenUnit={tokenUnit}
                onTokenUnitChange={setTokenUnit}
                showRechargePrice={showRechargePrice}
                onRechargePriceChange={setShowRechargePrice}
                viewMode={viewMode}
                onViewModeChange={setViewMode}
                quotaTypeFilter={quotaTypeFilter}
                endpointTypeFilter={endpointTypeFilter}
                vendorFilter={vendorFilter}
                groupFilter={groupFilter}
                tagFilter={tagFilter}
                onQuotaTypeChange={setQuotaTypeFilter}
                onEndpointTypeChange={setEndpointTypeFilter}
                onVendorChange={setVendorFilter}
                onGroupChange={setGroupFilter}
                onTagChange={setTagFilter}
                vendors={vendors || []}
                groups={availableGroups}
                groupRatios={groupRatio}
                tags={availableTags}
                models={models || []}
                hasActiveFilters={hasActiveFilters}
                activeFilterCount={activeFilterCount}
                onClearFilters={clearFilters}
              />

              {renderPricingContent()}
            </main>
          </div>

          {selectedModel && (
            <ModelDetailsDrawer
              open={Boolean(selectedModel)}
              onOpenChange={(open) => {
                if (!open) setSelectedModelName(null)
              }}
              model={selectedModel}
              groupRatio={groupRatio || {}}
              usableGroup={usableGroup || {}}
              endpointMap={
                (endpointMap as Record<
                  string,
                  { path?: string; method?: string }
                >) || {}
              }
              autoGroups={autoGroups || []}
              priceRate={priceRate ?? 1}
              usdExchangeRate={usdExchangeRate ?? 1}
              tokenUnit={tokenUnit}
              showRechargePrice={showRechargePrice}
            />
          )}
        </PageTransition>
      </div>
    </PublicLayout>
  )
}
