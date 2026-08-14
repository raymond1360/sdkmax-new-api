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
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Eye, EyeOff, RefreshCcw, RotateCcw, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getModelAvailability,
  triggerModelAvailabilityRetest,
  updateModelAvailabilityAdminEnabled,
} from '../api'
import { SettingsSection } from '../components/settings-section'
import type { ModelAvailability } from '../types'

const PAGE_SIZE = 50

const connectivityOptions = [
  'connected',
  'degraded',
  'failed',
  'wrong_endpoint',
  'wrong_payload',
  'batch_required',
  'unsupported',
  'upstream_missing',
  'rate_limited',
  'no_provider',
  'timeout',
  'upstream_error',
  'untested',
  'unknown',
]

const supportOptions = ['supported', 'unsupported', 'unverified']

function formatTime(timestamp: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

function getErrorMessage(error: unknown) {
  const err = error as {
    response?: { data?: { message?: string } }
    message?: string
  }
  return err?.response?.data?.message || err?.message || 'Request failed'
}

function getStatusVariant(status: string) {
  if (status === 'connected') return 'default'
  if (status === 'degraded' || status === 'rate_limited') return 'secondary'
  if (status === 'unknown' || status === 'untested') return 'outline'
  return 'destructive'
}

function getSupportVariant(status: string) {
  if (status === 'supported') return 'default'
  if (status === 'unsupported') return 'destructive'
  return 'outline'
}

function compactCapabilities(value: string) {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
    .slice(0, 3)
    .join(', ')
}

function ActionButtons({
  model,
  disabled,
  onToggle,
  onRetest,
}: {
  model: ModelAvailability
  disabled: boolean
  onToggle: (model: ModelAvailability) => void
  onRetest: (model: ModelAvailability) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex min-w-[168px] items-center justify-end gap-2'>
      <Button
        variant={model.admin_enabled ? 'destructive' : 'secondary'}
        size='sm'
        onClick={() => onToggle(model)}
        disabled={disabled}
        title={model.admin_enabled ? t('Hide model') : t('Enable model')}
      >
        {model.admin_enabled ? (
          <EyeOff data-icon='inline-start' />
        ) : (
          <Eye data-icon='inline-start' />
        )}
        {model.admin_enabled ? t('Hide') : t('Enable')}
      </Button>
      <Button
        variant='outline'
        size='sm'
        onClick={() => onRetest(model)}
        disabled={disabled}
        title={t('Request retest')}
      >
        <RotateCcw data-icon='inline-start' />
        {t('Retest')}
      </Button>
    </div>
  )
}

export function ModelAvailabilityCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [page, setPage] = useState(1)
  const [keyword, setKeyword] = useState('')
  const [connectivityStatus, setConnectivityStatus] = useState('')
  const [supportStatus, setSupportStatus] = useState('')
  const [customerVisible, setCustomerVisible] = useState('')

  useEffect(() => {
    setPage(1)
  }, [keyword, connectivityStatus, supportStatus, customerVisible])

  const queryParams = useMemo(
    () => ({
      page,
      page_size: PAGE_SIZE,
      keyword: keyword.trim() || undefined,
      connectivity_status: connectivityStatus || undefined,
      sdkmax_support_status: supportStatus || undefined,
      customer_visible: customerVisible || undefined,
    }),
    [customerVisible, keyword, connectivityStatus, page, supportStatus]
  )

  const availabilityQuery = useQuery({
    queryKey: ['model-availability', queryParams],
    queryFn: () => getModelAvailability(queryParams),
    meta: { skipGlobalErrorToast: true },
    retry: false,
  })

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['model-availability'] })

  const adminMutation = useMutation({
    mutationFn: ({
      modelID,
      adminEnabled,
    }: {
      modelID: string
      adminEnabled: boolean
    }) => updateModelAvailabilityAdminEnabled(modelID, adminEnabled),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Setting updated successfully'))
        invalidate()
      } else {
        toast.error(res.message || t('Failed to update setting'))
      }
    },
    onError: (error) => toast.error(t(getErrorMessage(error))),
  })

  const retestMutation = useMutation({
    mutationFn: triggerModelAvailabilityRetest,
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Retest requested'))
        invalidate()
      } else {
        toast.error(res.message || t('Request failed'))
      }
    },
    onError: (error) => toast.error(t(getErrorMessage(error))),
  })

  const rows = availabilityQuery.data?.data ?? []
  const total = availabilityQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const visibleCount = rows.filter((row) => row.customer_visible).length
  const hiddenCount = rows.length - visibleCount
  const hasPrevious = page > 1
  const hasNext = page < totalPages
  const isMutating = adminMutation.isPending || retestMutation.isPending

  const handleToggle = (model: ModelAvailability) => {
    const nextEnabled = !model.admin_enabled
    const action = nextEnabled ? t('Enable model') : t('Hide model')
    if (!window.confirm(`${action}: ${model.model_id}`)) return
    adminMutation.mutate({
      modelID: model.model_id,
      adminEnabled: nextEnabled,
    })
  }

  const handleRetest = (model: ModelAvailability) => {
    retestMutation.mutate(model.model_id)
  }

  return (
    <SettingsSection title={t('Model Health')}>
      <div className='space-y-4'>
        <div className='grid gap-3 lg:grid-cols-[minmax(220px,1fr)_repeat(3,minmax(150px,180px))_auto]'>
          <div className='relative'>
            <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2' />
            <Input
              value={keyword}
              onChange={(event) => setKeyword(event.currentTarget.value)}
              className='pl-8'
              placeholder={t('Search model')}
            />
          </div>
          <NativeSelect
            value={connectivityStatus}
            onChange={(event) =>
              setConnectivityStatus(event.currentTarget.value)
            }
          >
            <NativeSelectOption value=''>
              {t('All connectivity')}
            </NativeSelectOption>
            {connectivityOptions.map((status) => (
              <NativeSelectOption key={status} value={status}>
                {status}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <NativeSelect
            value={supportStatus}
            onChange={(event) => setSupportStatus(event.currentTarget.value)}
          >
            <NativeSelectOption value=''>{t('All support')}</NativeSelectOption>
            {supportOptions.map((status) => (
              <NativeSelectOption key={status} value={status}>
                {status}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <NativeSelect
            value={customerVisible}
            onChange={(event) => setCustomerVisible(event.currentTarget.value)}
          >
            <NativeSelectOption value=''>
              {t('All visibility')}
            </NativeSelectOption>
            <NativeSelectOption value='true'>{t('Visible')}</NativeSelectOption>
            <NativeSelectOption value='false'>{t('Hidden')}</NativeSelectOption>
          </NativeSelect>
          <Button
            variant='outline'
            onClick={() => availabilityQuery.refetch()}
            disabled={availabilityQuery.isFetching}
          >
            <RefreshCcw data-icon='inline-start' />
            {t('Refresh')}
          </Button>
        </div>

        <div className='flex flex-wrap items-center gap-2 text-sm'>
          <Badge variant='outline'>
            {t('Total')}: {total}
          </Badge>
          <Badge>
            {t('Visible')}: {visibleCount}
          </Badge>
          <Badge variant='destructive'>
            {t('Hidden')}: {hiddenCount}
          </Badge>
          <span className='text-muted-foreground ml-auto'>
            {t('Page')} {page} / {totalPages}
          </span>
        </div>

        <div className='overflow-hidden rounded-md border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('Model')}</TableHead>
                <TableHead>{t('Actions')}</TableHead>
                <TableHead>{t('Source')}</TableHead>
                <TableHead>{t('Mode')}</TableHead>
                <TableHead>{t('Capabilities')}</TableHead>
                <TableHead>{t('Connectivity')}</TableHead>
                <TableHead>{t('Support')}</TableHead>
                <TableHead>{t('Visibility')}</TableHead>
                <TableHead>{t('Reason')}</TableHead>
                <TableHead>{t('Counters')}</TableHead>
                <TableHead>{t('Last Tested')}</TableHead>
                <TableHead>{t('Last Error')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((model) => (
                <TableRow key={model.model_id}>
                  <TableCell
                    className='max-w-[260px] truncate font-medium'
                    title={model.model_id}
                  >
                    {model.model_id}
                  </TableCell>
                  <TableCell>
                    <ActionButtons
                      model={model}
                      disabled={isMutating}
                      onToggle={handleToggle}
                      onRetest={handleRetest}
                    />
                  </TableCell>
                  <TableCell>{model.source || '-'}</TableCell>
                  <TableCell>{model.api_mode || '-'}</TableCell>
                  <TableCell
                    className='max-w-[180px] truncate'
                    title={model.capabilities}
                  >
                    {compactCapabilities(model.capabilities) || '-'}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={getStatusVariant(model.connectivity_status)}
                    >
                      {model.connectivity_status || '-'}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={getSupportVariant(model.sdkmax_support_status)}
                    >
                      {model.sdkmax_support_status || '-'}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        model.customer_visible ? 'default' : 'destructive'
                      }
                    >
                      {model.customer_visible ? t('Visible') : t('Hidden')}
                    </Badge>
                  </TableCell>
                  <TableCell
                    className='max-w-[190px] truncate'
                    title={model.visibility_reason}
                  >
                    {model.visibility_reason || '-'}
                  </TableCell>
                  <TableCell>
                    {model.consecutive_failures}/
                    {model.consecutive_real_failures}/
                    {model.consecutive_successes}
                  </TableCell>
                  <TableCell>{formatTime(model.last_tested_at)}</TableCell>
                  <TableCell
                    className='max-w-[240px] truncate'
                    title={model.last_error || model.last_failure_reason}
                  >
                    {model.last_error || model.last_failure_reason || '-'}
                  </TableCell>
                </TableRow>
              ))}
              {!rows.length && (
                <TableRow>
                  <TableCell
                    colSpan={12}
                    className='text-muted-foreground h-24 text-center'
                  >
                    {availabilityQuery.isLoading
                      ? t('Loading')
                      : t('No results')}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>

        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious
                href='#'
                text={t('Previous')}
                aria-disabled={!hasPrevious}
                className={!hasPrevious ? 'pointer-events-none opacity-50' : ''}
                onClick={(event) => {
                  event.preventDefault()
                  if (hasPrevious) setPage((value) => value - 1)
                }}
              />
            </PaginationItem>
            <PaginationItem>
              <PaginationNext
                href='#'
                text={t('Next')}
                aria-disabled={!hasNext}
                className={!hasNext ? 'pointer-events-none opacity-50' : ''}
                onClick={(event) => {
                  event.preventDefault()
                  if (hasNext) setPage((value) => value + 1)
                }}
              />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      </div>
    </SettingsSection>
  )
}
