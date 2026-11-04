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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { ErrorState } from '@/components/error-state'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  contentSessionKey,
  useContentSession,
} from '@/features/usage-logs/hooks/use-content-session'
import { toIntlLocale } from '@/i18n/languages'
import { api } from '@/lib/api'
import { formatNumber } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { updateSystemOption } from '../api'
import {
  SettingsForm,
  SettingsFormGrid,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { safeNumberFieldProps } from '../utils/numeric-field'

const settingsSchema = (message: string) =>
  z.object({
    enabled: z.boolean({ error: message }),
    retention_days: z
      .number({ error: message })
      .int(message)
      .min(1, message)
      .max(365, message),
    max_body_bytes: z
      .number({ error: message })
      .int(message)
      .min(1024, message)
      .max(64 * 1024 * 1024, message),
  })
type ContentSettings = z.infer<ReturnType<typeof settingsSchema>>

type ContentStatus = {
  configured: boolean
  ready: boolean
  settings: ContentSettings
  capture: { active: number; skipped: number; dropped: number }
  publisher: {
    failed: number
    dropped: number
    delivered: number
    queued_bytes: number
  }
}

function RequestContentSettings(props: { sessionKey: string }) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const status = useQuery({
    queryKey: ['request-content-status', props.sessionKey],
    queryFn: async ({ signal }) => {
      const response = await api.get('/api/option/request_content/status', {
        signal,
        disableDuplicate: true,
      })
      if (signal.aborted || contentSessionKey() !== props.sessionKey) {
        throw new Error('Session changed')
      }
      return requireServerSuccess(response.data).data as ContentStatus
    },
    refetchInterval: 30_000,
    retry: false,
    gcTime: 0,
    meta: { errorToast: false },
  })
  const form = useForm<ContentSettings>({
    resolver: zodResolver(
      settingsSchema(t('Invalid request content settings'))
    ),
    defaultValues: {
      enabled: false,
      retention_days: 7,
      max_body_bytes: 1048576,
    },
  })
  const settings = status.data?.settings
  useEffect(() => {
    if (settings && !form.formState.isDirty) form.reset(settings)
  }, [settings, form])

  const save = useMutation({
    mutationFn: async (values: ContentSettings) => {
      if (!status.data || contentSessionKey() !== props.sessionKey) {
        throw new Error('Session changed')
      }
      if (values.enabled && !status.data.ready) {
        throw new Error('Request content retention is not configured')
      }
      // Stop first when disabling; apply limits before enabling new recordings.
      const fields: (keyof ContentSettings)[] = values.enabled
        ? ['retention_days', 'max_body_bytes', 'enabled']
        : ['enabled', 'retention_days', 'max_body_bytes']
      for (const field of fields) {
        if (values[field] === status.data.settings[field]) continue
        if (contentSessionKey() !== props.sessionKey) {
          throw new Error('Session changed')
        }
        requireServerSuccess(
          await updateSystemOption({
            key: `request_content.${field}`,
            value: values[field],
          })
        )
      }
      return values
    },
    onSuccess: (values) => {
      if (contentSessionKey() !== props.sessionKey) return
      form.reset(values)
      void client.invalidateQueries({
        queryKey: ['request-content-status', props.sessionKey],
      })
      void client.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Setting updated successfully'))
    },
    onError: (error) => {
      if (contentSessionKey() !== props.sessionKey) return
      handleServerError(error, t('Failed to update setting'))
      void client.invalidateQueries({
        queryKey: ['request-content-status', props.sessionKey],
      })
    },
  })
  const submit = form.handleSubmit((values) => save.mutate(values))
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  if (status.isPending) {
    return <LoadingState message={t('Loading retention settings...')} />
  }
  if (status.isError || !status.data) {
    return <ErrorState onRetry={() => void status.refetch()} />
  }

  return (
    <SettingsSection title={t('Request content retention')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Retain request and response bodies for Chat Completions, Responses, and Messages, including streaming output. Only root can view retained content.'
        )}
      </p>
      {!status.data.ready && (
        <Alert>
          <AlertDescription>
            {t(
              'Configure the content retention service before enabling new recordings.'
            )}
          </AlertDescription>
        </Alert>
      )}
      <Form {...form}>
        <SettingsForm onSubmit={submit}>
          <SettingsPageFormActions
            onSave={submit}
            isSaving={save.isPending}
            saveLabel='Save retention settings'
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable request content retention')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Turning this off stops new recordings. Existing content remains available until it expires.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={
                      save.isPending || (!status.data?.ready && !field.value)
                    }
                  />
                </FormControl>
                <FormMessage />
              </SettingsSwitchItem>
            )}
          />
          <SettingsFormGrid>
            <FormField
              control={form.control}
              name='retention_days'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Content retention (days)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={365}
                      step={1}
                      disabled={save.isPending}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='max_body_bytes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Maximum retained bytes per body')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1024}
                      max={64 * 1024 * 1024}
                      step={1024}
                      disabled={save.isPending}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Applies separately to requests and responses. Content beyond this limit is marked as truncated; the API response continues normally.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </SettingsFormGrid>
        </SettingsForm>
      </Form>
      <div
        className='grid gap-3 rounded-lg border p-4 text-sm sm:grid-cols-2 xl:grid-cols-3'
        aria-label={t('Content retention status')}
      >
        <div>
          <span className='text-muted-foreground'>
            {t('Active recordings')}
          </span>
          <p>{formatNumber(status.data.capture?.active ?? 0, locale)}</p>
        </div>
        <div>
          <span className='text-muted-foreground'>
            {t('Skipped recordings')}
          </span>
          <p>{formatNumber(status.data.capture?.skipped ?? 0, locale)}</p>
        </div>
        <div>
          <span className='text-muted-foreground'>
            {t('Content delivery failures')}
          </span>
          <p>{formatNumber(status.data.publisher?.failed ?? 0, locale)}</p>
        </div>
        <div>
          <span className='text-muted-foreground'>
            {t('Capture events dropped')}
          </span>
          <p>{formatNumber(status.data.capture?.dropped ?? 0, locale)}</p>
        </div>
        <div>
          <span className='text-muted-foreground'>
            {t('Delivery queue events dropped')}
          </span>
          <p>{formatNumber(status.data.publisher?.dropped ?? 0, locale)}</p>
        </div>
      </div>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Counters apply to this gateway process. They do not determine whether an individual request was archived.'
        )}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t(
          'Dropped event counters may overlap and must not be added together. They count content segments, not lost requests.'
        )}
      </p>
    </SettingsSection>
  )
}

export function RequestContentSection() {
  const sessionKey = useContentSession()
  if (!sessionKey) return null
  return <RequestContentSettings key={sessionKey} sessionKey={sessionKey} />
}
