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
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { IconGoogle } from '@/assets/brand-icons'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { useOAuthLogin } from '../hooks/use-oauth-login'
import type { SystemStatus } from '../types'

type GoogleSignInButtonProps = {
  status: SystemStatus | null
  disabled?: boolean
  className?: string
}

/**
 * Primary "Continue with Google" call to action. Google is configured as a
 * custom OAuth provider (provider_type: 'google') so it can appear in
 * status.custom_oauth_providers like any other custom provider, but it gets
 * its own prominent, brand-colored button here instead of the generic
 * secondary-style list in <OAuthProviders>. Callers are responsible for
 * excluding the Google entry from that generic list to avoid a duplicate
 * button.
 */
export function GoogleSignInButton({
  status,
  disabled = false,
  className,
}: GoogleSignInButtonProps) {
  const { t } = useTranslation()
  const { isLoading, googleProvider, handleGoogleLogin } = useOAuthLogin(status)

  if (!googleProvider) return null

  return (
    <Button
      type='button'
      variant='default'
      disabled={disabled || isLoading}
      onClick={() => handleGoogleLogin(googleProvider)}
      className={cn(
        'h-11 w-full justify-center gap-2 rounded-lg bg-white text-gray-700 shadow-sm hover:bg-gray-50 dark:bg-white dark:text-gray-700 dark:hover:bg-gray-100',
        'border border-gray-300',
        className
      )}
    >
      {isLoading ? (
        <Loader2 className='h-4 w-4 animate-spin' />
      ) : (
        <IconGoogle className='h-5 w-5' />
      )}
      {t('Continue with your Google account')}
    </Button>
  )
}
