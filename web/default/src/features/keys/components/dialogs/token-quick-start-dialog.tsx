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
import { useMemo, useState } from 'react'
import { Check, Eye, EyeOff, Terminal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/use-status'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CopyButton } from '@/components/copy-button'
import { getSdkmaxApiBaseUrl } from '../../constants'

type Provider = 'openai' | 'anthropic' | 'gemini'
type Language = 'python' | 'node' | 'curl'

interface TokenQuickStartDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  apiKey: string
}

const PROVIDERS: Array<{ value: Provider; label: string; model: string }> = [
  { value: 'openai', label: 'OpenAI', model: 'gpt-4o' },
  {
    value: 'anthropic',
    label: 'Anthropic',
    model: 'claude-3-5-sonnet-20241022',
  },
  { value: 'gemini', label: 'Gemini', model: 'gemini-1.5-pro' },
]

const LANGUAGES: Array<{ value: Language; label: string }> = [
  { value: 'python', label: 'Python' },
  { value: 'node', label: 'Node.js' },
  { value: 'curl', label: 'cURL' },
]

function normalizeApiKey(apiKey: string): string {
  if (!apiKey) return ''
  return apiKey.startsWith('sk-') ? apiKey : `sk-${apiKey}`
}

function buildCodeSample(
  provider: Provider,
  language: Language,
  apiKey: string,
  baseUrl: string
) {
  const model =
    PROVIDERS.find((item) => item.value === provider)?.model ?? 'gpt-4o'

  if (language === 'python') {
    return [
      'from openai import OpenAI',
      '',
      'client = OpenAI(',
      `    api_key="${apiKey || 'YOUR_API_KEY'}",`,
      `    base_url="${baseUrl}"`,
      ')',
      '',
      'response = client.chat.completions.create(',
      `    model="${model}",`,
      '    messages=[',
      '        {',
      '            "role": "user",',
      '            "content": "你好"',
      '        }',
      '    ]',
      ')',
      '',
      'print(response)',
    ].join('\n')
  }

  if (language === 'node') {
    return [
      "import OpenAI from 'openai'",
      '',
      'const client = new OpenAI({',
      `  apiKey: '${apiKey || 'YOUR_API_KEY'}',`,
      `  baseURL: '${baseUrl}',`,
      '})',
      '',
      'const response = await client.chat.completions.create({',
      `  model: '${model}',`,
      "  messages: [{ role: 'user', content: '你好' }],",
      '})',
      '',
      'console.log(response)',
    ].join('\n')
  }

  return [
    `curl ${baseUrl}/chat/completions \\`,
    `  -H "Authorization: Bearer ${apiKey || 'YOUR_API_KEY'}" \\`,
    '  -H "Content-Type: application/json" \\',
    "  -d '{",
    `    "model": "${model}",`,
    '    "messages": [',
    '      {',
    '        "role": "user",',
    '        "content": "你好"',
    '      }',
    '    ]',
    "  }'",
  ].join('\n')
}

function CodeBlock({
  code,
  copyTooltip,
}: {
  code: string
  copyTooltip: string
}) {
  return (
    <div className='bg-muted/50 relative overflow-hidden rounded-md border'>
      <CopyButton
        value={code}
        tooltip={copyTooltip}
        className='bg-background/80 absolute top-2 right-2 backdrop-blur'
      />
      <pre className='max-h-[360px] overflow-auto p-3 pr-12 text-xs leading-relaxed'>
        <code>{code}</code>
      </pre>
    </div>
  )
}

export function TokenQuickStartDialog({
  open,
  onOpenChange,
  apiKey,
}: TokenQuickStartDialogProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [showKey, setShowKey] = useState(false)
  const [provider, setProvider] = useState<Provider>('openai')
  const [language, setLanguage] = useState<Language>('python')
  const fullApiKey = normalizeApiKey(apiKey)
  const serverAddress =
    typeof status?.server_address === 'string'
      ? status.server_address
      : undefined
  const baseUrl = getSdkmaxApiBaseUrl(serverAddress)
  const sampleApiKey = showKey ? fullApiKey : 'YOUR_API_KEY'
  const maskedKey = fullApiKey
    ? `${fullApiKey.slice(0, 7)}${'•'.repeat(18)}${fullApiKey.slice(-6)}`
    : ''

  const code = useMemo(
    () => buildCodeSample(provider, language, sampleApiKey, baseUrl),
    [provider, language, sampleApiKey, baseUrl]
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <Check className='text-success size-4' />
            {t('API Key created successfully')}
          </DialogTitle>
          <DialogDescription>
            {t('Use this key with SDKMAX compatible API examples.')}
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-1.5'>
              <Label>{t('API Key')}</Label>
              <div className='flex min-w-0 items-center gap-2 rounded-md border px-3 py-2'>
                <code className='min-w-0 flex-1 truncate text-xs'>
                  {showKey ? fullApiKey : maskedKey}
                </code>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  onClick={() => setShowKey((value) => !value)}
                  aria-label={showKey ? t('Hide') : t('Show')}
                >
                  {showKey ? (
                    <EyeOff className='size-4' />
                  ) : (
                    <Eye className='size-4' />
                  )}
                </Button>
                <CopyButton value={fullApiKey} tooltip={t('Copy API Key')} />
              </div>
            </div>

            <div className='space-y-1.5'>
              <Label>{t('Base URL')}</Label>
              <div className='flex min-w-0 items-center gap-2 rounded-md border px-3 py-2'>
                <code className='min-w-0 flex-1 truncate text-xs'>
                  {baseUrl}
                </code>
                <CopyButton value={baseUrl} tooltip={t('Copy Base URL')} />
              </div>
            </div>
          </div>

          <div className='space-y-3'>
            <div className='flex items-center gap-2 text-sm font-medium'>
              <Terminal className='size-4' />
              {t('Quick start examples')}
            </div>
            {!showKey && (
              <p className='text-muted-foreground text-xs'>
                {t('API Key is hidden in examples. Click show to include it.')}
              </p>
            )}

            <Tabs
              value={provider}
              onValueChange={(value) => setProvider(value as Provider)}
            >
              <TabsList className='grid w-full grid-cols-3'>
                {PROVIDERS.map((item) => (
                  <TabsTrigger key={item.value} value={item.value}>
                    {item.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>

            <Tabs
              value={language}
              onValueChange={(value) => setLanguage(value as Language)}
            >
              <TabsList className='grid w-full grid-cols-3'>
                {LANGUAGES.map((item) => (
                  <TabsTrigger key={item.value} value={item.value}>
                    {item.label}
                  </TabsTrigger>
                ))}
              </TabsList>
              {LANGUAGES.map((item) => (
                <TabsContent
                  key={item.value}
                  value={item.value}
                  className={cn(item.value === language ? 'block' : 'hidden')}
                >
                  <CodeBlock code={code} copyTooltip={t('Copy code')} />
                </TabsContent>
              ))}
            </Tabs>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('Done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
