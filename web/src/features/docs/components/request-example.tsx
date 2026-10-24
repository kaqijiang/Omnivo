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
import { useTranslation } from 'react-i18next'

import { CodeBlockFrame } from '@/components/ai-elements/code-block'
import { CopyButton } from '@/components/copy-button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { buildRequestExamples } from '../request-examples'

type RequestExampleProps = {
  baseUrl: string
}

export function RequestExample(props: RequestExampleProps) {
  const { t } = useTranslation()
  const examples = buildRequestExamples(props.baseUrl)
  const languages = [
    { value: 'curl', name: 'curl', install: '', code: examples.curl },
    {
      value: 'python',
      name: 'Python',
      install: 'python -m pip install openai',
      code: examples.python,
    },
    {
      value: 'javascript',
      name: 'Node.js',
      install: 'npm install openai',
      code: examples.javascript,
    },
  ]

  return (
    <Tabs defaultValue='curl' className='min-w-0'>
      <TabsList aria-label={t('Example language')}>
        {languages.map((language) => (
          <TabsTrigger key={language.value} value={language.value}>
            {language.name}
          </TabsTrigger>
        ))}
      </TabsList>
      {languages.map((language) => (
        <TabsContent key={language.value} value={language.value}>
          {language.install && (
            <p className='text-muted-foreground mt-3 text-sm'>
              {t('Install the SDK:')} <code>{language.install}</code>
            </p>
          )}
          <CodeBlockFrame
            title={t('Request example')}
            showToolbar
            endActions={
              <CopyButton
                value={language.code}
                aria-label={t('Copy request example')}
              />
            }
          >
            <pre className='p-5 text-xs leading-7 sm:text-sm'>
              <code>{language.code}</code>
            </pre>
          </CodeBlockFrame>
        </TabsContent>
      ))}
    </Tabs>
  )
}
