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
import { RefreshCcw, Save } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getOpenRouterChannels,
  getOpenRouterSyncState,
  triggerOpenRouterSync,
  updateOpenRouterGlobalMultiplier,
  updateOpenRouterModelMultiplier,
  updateOpenRouterUnifiedChannel,
} from '../api'
import { SettingsSection } from '../components/settings-section'

function formatTime(timestamp: number) {
  if (!timestamp) return '-'
  return new Date(timestamp * 1000).toLocaleString()
}

function formatUsd(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '$0.00'
  const maximumFractionDigits = value >= 1 ? 2 : 6
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits,
  }).format(value)
}

function formatPricePerMillion(inputPrice: string, outputPrice: string) {
  const input = Number(inputPrice) * 1_000_000
  const output = Number(outputPrice) * 1_000_000
  return `${formatUsd(input)} / ${formatUsd(output)}`
}

function getSdkmaxDisplayPrice(
  inputPrice: string,
  outputPrice: string,
  globalMultiplier: string,
  modelMultiplier: string
) {
  const global = Number(globalMultiplier)
  const model = Number(modelMultiplier || '1')
  const multiplier =
    Number.isFinite(global) && global > 0 && Number.isFinite(model) && model > 0
      ? global * model
      : 1
  return formatPricePerMillion(
    String(Number(inputPrice) * multiplier),
    String(Number(outputPrice) * multiplier)
  )
}

function getErrorMessage(error: unknown) {
  const err = error as {
    status?: number
    response?: { status?: number; data?: { message?: string } }
    message?: string
  }
  const status = err?.response?.status ?? err?.status
  if (status === 401 || status === 403) {
    return '登录态已失效或当前账号无权限访问 OpenRouter Sync，请重新登录管理员账号。'
  }
  if (status === 404) {
    return 'OpenRouter Sync API 不可用，请使用最新代码重建或重启后端服务。'
  }
  const message = err?.response?.data?.message || err?.message
  if (message) return status ? `${message} (${status})` : message
  return status ? `Request failed (${status})` : 'Request failed'
}

export function OpenRouterSyncCard() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [globalMultiplier, setGlobalMultiplier] = useState('1.00')

  const stateQuery = useQuery({
    queryKey: ['openrouter-sync-state'],
    queryFn: getOpenRouterSyncState,
    meta: { skipGlobalErrorToast: true },
    retry: false,
  })
  const channelsQuery = useQuery({
    queryKey: ['openrouter-sync-channels'],
    queryFn: getOpenRouterChannels,
    meta: { skipGlobalErrorToast: true },
    retry: false,
  })

  const data = stateQuery.data?.data
  const allowed = data?.settings.allowed_multipliers ?? [
    '0.90',
    '1.00',
    '1.05',
    '1.10',
    '1.30',
  ]

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ['openrouter-sync-state'] })
    queryClient.invalidateQueries({ queryKey: ['openrouter-sync-channels'] })
    queryClient.invalidateQueries({ queryKey: ['system-options'] })
  }

  const syncMutation = useMutation({
    mutationFn: triggerOpenRouterSync,
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('OpenRouter models synced successfully'))
        invalidate()
      } else {
        toast.error(res.message || t('Failed to sync OpenRouter models'))
      }
    },
    onError: (error) => {
      toast.error(t(getErrorMessage(error)))
    },
  })

  const globalMutation = useMutation({
    mutationFn: updateOpenRouterGlobalMultiplier,
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Setting updated successfully'))
        invalidate()
      } else {
        toast.error(res.message || t('Failed to update setting'))
      }
    },
    onError: (error) => {
      toast.error(t(getErrorMessage(error)))
    },
  })

  const modelMutation = useMutation({
    mutationFn: ({
      modelID,
      multiplier,
    }: {
      modelID: string
      multiplier: string
    }) => updateOpenRouterModelMultiplier(modelID, multiplier),
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Setting updated successfully'))
        invalidate()
      } else {
        toast.error(res.message || t('Failed to update setting'))
      }
    },
    onError: (error) => {
      toast.error(t(getErrorMessage(error)))
    },
  })

  const channelMutation = useMutation({
    mutationFn: updateOpenRouterUnifiedChannel,
    onSuccess: (res) => {
      if (res.success) {
        toast.success(t('Setting updated successfully'))
        invalidate()
      } else {
        toast.error(res.message || t('Failed to update setting'))
      }
    },
    onError: (error) => {
      toast.error(t(getErrorMessage(error)))
    },
  })

  const models = data?.models ?? []
  const logs = data?.logs ?? []
  const channels = channelsQuery.data?.data ?? []
  const selectedChannel = Number(data?.settings.unified_channel_id ?? 0)

  const latestLog = useMemo(() => logs[0], [logs])
  const apiUnavailableMessage = stateQuery.isError
    ? getErrorMessage(stateQuery.error)
    : ''

  useEffect(() => {
    if (data?.settings.global_multiplier) {
      setGlobalMultiplier(data.settings.global_multiplier)
    }
  }, [data?.settings.global_multiplier])

  const handleSaveGlobalMultiplier = () => {
    const parsed = Number(globalMultiplier)
    if (!Number.isFinite(parsed) || parsed < 0.01 || parsed > 10) {
      toast.error(t('Multiplier must be between 0.01 and 10.00'))
      return
    }
    globalMutation.mutate(parsed.toFixed(2))
  }

  return (
    <SettingsSection title={t('OpenRouter Sync')}>
      {apiUnavailableMessage && (
        <Alert variant='destructive'>
          <AlertTitle>{t('OpenRouter Sync 暂不可用')}</AlertTitle>
          <AlertDescription>{t(apiUnavailableMessage)}</AlertDescription>
        </Alert>
      )}
      <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(260px,360px)]'>
        <div className='space-y-4'>
          <div className='flex flex-wrap items-center gap-3'>
            <div className='flex items-center gap-2'>
              <Input
                className='w-24'
                type='number'
                min='0.01'
                max='10'
                step='0.01'
                value={globalMultiplier}
                onChange={(event) =>
                  setGlobalMultiplier(event.currentTarget.value)
                }
                disabled={globalMutation.isPending}
                aria-label={t('Global multiplier')}
              />
              <Button
                variant='secondary'
                onClick={handleSaveGlobalMultiplier}
                disabled={globalMutation.isPending}
              >
                <Save className='mr-2 h-4 w-4' />
                {t('Save')}
              </Button>
            </div>
            <NativeSelect
              value={selectedChannel}
              onChange={(event) =>
                channelMutation.mutate(Number(event.currentTarget.value))
              }
              disabled={channelMutation.isPending}
            >
              <NativeSelectOption value={0}>
                {t('Auto select OpenRouter channel')}
              </NativeSelectOption>
              {channels.map((channel) => (
                <NativeSelectOption key={channel.id} value={channel.id}>
                  #{channel.id} {channel.name}
                </NativeSelectOption>
              ))}
            </NativeSelect>
            <Button
              onClick={() => syncMutation.mutate()}
              disabled={syncMutation.isPending}
            >
              <RefreshCcw className='mr-2 h-4 w-4' />
              {syncMutation.isPending ? t('Syncing') : t('Sync Now')}
            </Button>
          </div>

          <div className='overflow-hidden rounded-md border'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('Model')}</TableHead>
                  <TableHead>{t('Provider')}</TableHead>
                  <TableHead>{t('Context')}</TableHead>
                  <TableHead>{t('OpenRouter Price')}</TableHead>
                  <TableHead>{t('SDKMAX Price')}</TableHead>
                  <TableHead>{t('Multiplier')}</TableHead>
                  <TableHead>{t('Health')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {models.map((model) => (
                  <TableRow key={model.model_id}>
                    <TableCell className='max-w-[280px] truncate font-medium'>
                      {model.model_id}
                    </TableCell>
                    <TableCell>{model.provider || '-'}</TableCell>
                    <TableCell>{model.context_length || '-'}</TableCell>
                    <TableCell
                      className='whitespace-nowrap'
                      title={`${model.openrouter_input_price} / ${model.openrouter_output_price} per token`}
                    >
                      <span className='font-medium'>
                        {formatPricePerMillion(
                          model.openrouter_input_price,
                          model.openrouter_output_price
                        )}
                      </span>
                      <span className='text-muted-foreground ml-1 text-xs'>
                        per 1M
                      </span>
                    </TableCell>
                    <TableCell
                      className='whitespace-nowrap'
                      title={`${model.sdkmax_input_price} / ${model.sdkmax_output_price} per token`}
                    >
                      <span className='font-medium'>
                        {getSdkmaxDisplayPrice(
                          model.openrouter_input_price,
                          model.openrouter_output_price,
                          globalMultiplier,
                          model.model_multiplier || '1.00'
                        )}
                      </span>
                      <span className='text-muted-foreground ml-1 text-xs'>
                        per 1M
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className='flex items-center gap-2'>
                        <NativeSelect
                          size='sm'
                          value={model.model_multiplier || '1.00'}
                          onChange={(event) =>
                            modelMutation.mutate({
                              modelID: model.model_id,
                              multiplier: event.currentTarget.value,
                            })
                          }
                          disabled={modelMutation.isPending}
                        >
                          {allowed.map((value) => (
                            <NativeSelectOption key={value} value={value}>
                              {value}
                            </NativeSelectOption>
                          ))}
                        </NativeSelect>
                        <Save className='text-muted-foreground h-4 w-4' />
                      </div>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={model.healthy ? 'default' : 'destructive'}
                      >
                        {model.healthy ? t('Healthy') : t('Unavailable')}
                      </Badge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>

        <div className='space-y-3 rounded-md border p-4'>
          <div className='text-sm font-medium'>{t('Sync Logs')}</div>
          {latestLog ? (
            <div className='space-y-1 text-sm'>
              <div>
                <span className='text-muted-foreground'>{t('Latest')}:</span>{' '}
                {latestLog.status}
              </div>
              <div>
                <span className='text-muted-foreground'>{t('Fetched')}:</span>{' '}
                {latestLog.models_fetched}
              </div>
              <div>
                <span className='text-muted-foreground'>{t('Updated')}:</span>{' '}
                {latestLog.models_updated}
              </div>
              <div>
                <span className='text-muted-foreground'>{t('Time')}:</span>{' '}
                {formatTime(latestLog.finished_at || latestLog.started_at)}
              </div>
            </div>
          ) : (
            <div className='text-muted-foreground text-sm'>
              {t('No sync logs')}
            </div>
          )}
          <div className='max-h-80 space-y-2 overflow-auto pt-2'>
            {logs.map((log) => (
              <div key={log.id} className='border-t pt-2 text-xs'>
                <div className='font-medium'>{log.status}</div>
                <div className='text-muted-foreground'>
                  {formatTime(log.finished_at || log.started_at)}
                </div>
                {log.message && (
                  <div className='mt-1 break-words'>{log.message}</div>
                )}
              </div>
            ))}
          </div>
        </div>
      </div>
    </SettingsSection>
  )
}
