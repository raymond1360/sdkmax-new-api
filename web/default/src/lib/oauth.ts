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
import { api } from './api'

// ============================================================================
// Shared provider shape (subset needed for URL building/binding)
// ============================================================================

export interface OAuthCustomProviderLike {
  slug: string
  client_id: string
  authorization_endpoint: string
  scopes?: string
  provider_type?: string
}

// ============================================================================
// OAuth URL Builders
// ============================================================================

/**
 * Build GitHub OAuth URL
 */
export function buildGitHubOAuthUrl(clientId: string, state: string): string {
  return `https://github.com/login/oauth/authorize?client_id=${clientId}&state=${state}&scope=user:email`
}

/**
 * Build Discord OAuth URL
 */
export function buildDiscordOAuthUrl(clientId: string, state: string): string {
  const url = new URL('https://discord.com/oauth2/authorize')
  url.searchParams.set('client_id', clientId)
  url.searchParams.set(
    'redirect_uri',
    `${window.location.origin}/oauth/discord`
  )
  url.searchParams.set('response_type', 'code')
  url.searchParams.set('scope', 'identify+openid')
  url.searchParams.set('state', state)
  return url.toString()
}

/**
 * Build OIDC OAuth URL
 */
export function buildOIDCOAuthUrl(
  authUrl: string,
  clientId: string,
  state: string
): string {
  const url = new URL(authUrl)
  url.searchParams.set('client_id', clientId)
  url.searchParams.set('redirect_uri', `${window.location.origin}/oauth/oidc`)
  url.searchParams.set('response_type', 'code')
  url.searchParams.set('scope', 'openid profile email')
  url.searchParams.set('state', state)
  return url.toString()
}

/**
 * Build LinuxDO OAuth URL
 */
export function buildLinuxDOOAuthUrl(clientId: string, state: string): string {
  return `https://connect.linux.do/oauth2/authorize?response_type=code&client_id=${clientId}&state=${state}`
}

/**
 * Build the authorize URL for any custom OAuth provider (including Google,
 * which is configured as a custom provider with provider_type: 'google').
 * When nonce is provided it is included so ID-Token-verifying providers
 * (Google) can bind the returned token to this request.
 */
export function buildCustomOAuthUrl(
  provider: OAuthCustomProviderLike,
  state: string,
  nonce?: string
): string {
  const redirectUri = `${window.location.origin}/oauth/${provider.slug}`
  const url = new URL(provider.authorization_endpoint)
  url.searchParams.set('client_id', provider.client_id)
  url.searchParams.set('redirect_uri', redirectUri)
  url.searchParams.set('response_type', 'code')
  url.searchParams.set('state', state)
  if (provider.scopes) {
    url.searchParams.set('scope', provider.scopes)
  }
  if (nonce) {
    url.searchParams.set('nonce', nonce)
  }
  return url.toString()
}

// ============================================================================
// OAuth Helper Functions
// ============================================================================

/**
 * Get OAuth state token
 * Includes affiliate code from localStorage if available
 */
export async function getOAuthState(): Promise<string | null> {
  try {
    let path = '/api/oauth/state'
    const affCode = localStorage.getItem('aff')
    if (affCode && affCode.length > 0) {
      path += `?aff=${affCode}`
    }
    const res = await api.get(path)
    if (res.data.success) {
      return res.data.data
    }
    return null
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to get OAuth state:', error)
    return null
  }
}

/**
 * Get OAuth state token together with its nonce (only Google needs the
 * nonce; other providers ignore it). Same aff-code behavior as
 * getOAuthState().
 */
export async function getOAuthStateAndNonce(): Promise<{
  state: string
  nonce: string
} | null> {
  try {
    let path = '/api/oauth/state'
    const affCode = localStorage.getItem('aff')
    if (affCode && affCode.length > 0) {
      path += `?aff=${affCode}`
    }
    const res = await api.get(path)
    if (res.data.success && res.data.data) {
      return {
        state: res.data.data as string,
        nonce: (res.data.nonce as string) || '',
      }
    }
    return null
  } catch (error) {
    // eslint-disable-next-line no-console
    console.error('Failed to get OAuth state:', error)
    return null
  }
}

/**
 * Bind a custom OAuth provider (including Google) to the currently signed
 * in user. Unlike the login flow, this must NOT reset the current session:
 * binding relies on the backend seeing an authenticated session when the
 * callback runs, so the OAuth dance happens in a new tab (matching how
 * GitHub/Discord/OIDC/LinuxDO binding already works below).
 */
export async function handleCustomOAuthBind(
  provider: OAuthCustomProviderLike
): Promise<void> {
  const isGoogle = provider.provider_type === 'google'
  const stateInfo = isGoogle ? await getOAuthStateAndNonce() : null
  const state = isGoogle ? stateInfo?.state : await getOAuthState()
  if (!state) return

  const url = buildCustomOAuthUrl(provider, state, stateInfo?.nonce)
  window.open(url, '_blank')
}

/**
 * Handle GitHub OAuth binding/login
 */
export async function handleGitHubOAuth(clientId: string): Promise<void> {
  const state = await getOAuthState()
  if (!state) return

  const url = buildGitHubOAuthUrl(clientId, state)
  window.open(url, '_blank')
}

/**
 * Handle Discord OAuth binding/login
 */
export async function handleDiscordOAuth(clientId: string): Promise<void> {
  const state = await getOAuthState()
  if (!state) return

  const url = buildDiscordOAuthUrl(clientId, state)
  window.open(url, '_blank')
}

/**
 * Handle OIDC OAuth binding/login
 */
export async function handleOIDCOAuth(
  authUrl: string,
  clientId: string
): Promise<void> {
  const state = await getOAuthState()
  if (!state) return

  const url = buildOIDCOAuthUrl(authUrl, clientId, state)
  window.open(url, '_blank')
}

/**
 * Handle LinuxDO OAuth binding/login
 */
export async function handleLinuxDOOAuth(clientId: string): Promise<void> {
  const state = await getOAuthState()
  if (!state) return

  const url = buildLinuxDOOAuthUrl(clientId, state)
  window.open(url, '_blank')
}
