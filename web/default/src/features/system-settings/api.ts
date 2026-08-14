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
import { api } from '@/lib/api'
import type {
  ConfirmPaymentComplianceResponse,
  DeleteLogsResponse,
  FetchUpstreamRatiosRequest,
  ModelAvailabilityActionResponse,
  ModelAvailabilityListParams,
  ModelAvailabilityListResponse,
  OpenRouterChannelsResponse,
  OpenRouterSyncResponse,
  OpenRouterSyncStateResponse,
  SystemOptionsResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function deleteLogsBefore(targetTimestamp: number) {
  const res = await api.delete<DeleteLogsResponse>('/api/log/', {
    params: { target_timestamp: targetTimestamp },
  })
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}

export async function getOpenRouterSyncState() {
  const res = await api.get<OpenRouterSyncStateResponse>(
    '/api/openrouter_sync/',
    {
      params: { _t: Date.now() },
      skipErrorHandler: true,
      disableDuplicate: true,
    }
  )
  return res.data
}

export async function triggerOpenRouterSync() {
  const res = await api.post<OpenRouterSyncResponse>(
    '/api/openrouter_sync/sync',
    undefined,
    {
      skipErrorHandler: true,
    }
  )
  return res.data
}

export async function updateOpenRouterGlobalMultiplier(multiplier: string) {
  const res = await api.put<UpdateOptionResponse>(
    '/api/openrouter_sync/global_multiplier',
    { multiplier },
    {
      skipErrorHandler: true,
    }
  )
  return res.data
}

export async function updateOpenRouterModelMultiplier(
  model_id: string,
  multiplier: string
) {
  const res = await api.put<UpdateOptionResponse>(
    '/api/openrouter_sync/model_multiplier',
    { model_id, multiplier },
    {
      skipErrorHandler: true,
    }
  )
  return res.data
}

export async function getOpenRouterChannels() {
  const res = await api.get<OpenRouterChannelsResponse>(
    '/api/openrouter_sync/channels',
    {
      params: { _t: Date.now() },
      skipErrorHandler: true,
      disableDuplicate: true,
    }
  )
  return res.data
}

export async function updateOpenRouterUnifiedChannel(channel_id: number) {
  const res = await api.put<UpdateOptionResponse>(
    '/api/openrouter_sync/channel',
    { channel_id },
    {
      skipErrorHandler: true,
    }
  )
  return res.data
}

export async function getModelAvailability(
  params: ModelAvailabilityListParams
) {
  const res = await api.get<ModelAvailabilityListResponse>(
    '/api/model_availability/',
    {
      params: { ...params, _t: Date.now() },
      skipErrorHandler: true,
      disableDuplicate: true,
    }
  )
  return res.data
}

export async function updateModelAvailabilityAdminEnabled(
  model_id: string,
  admin_enabled: boolean
) {
  const res = await api.post<ModelAvailabilityActionResponse>(
    '/api/model_availability/admin_enabled',
    { model_id, admin_enabled },
    {
      skipErrorHandler: true,
    }
  )
  return res.data
}

export async function triggerModelAvailabilityRetest(model_id: string) {
  const res = await api.post<ModelAvailabilityActionResponse>(
    '/api/model_availability/retest',
    { model_id },
    {
      skipErrorHandler: true,
    }
  )
  return res.data
}
