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
import { useEffect, useState } from 'react'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

export function contentSessionKey(): string | null {
  const auth = useAuthStore.getState().auth
  if (
    auth.user?.role !== ROLE.SUPER_ADMIN ||
    auth.user.status === 2 ||
    !auth.accessToken ||
    !auth.session?.sid ||
    auth.session.expires_at * 1000 <= Date.now()
  ) {
    return null
  }
  return `${auth.user.id}:${auth.session.sid}`
}

export function useContentSession(): string | null {
  const user = useAuthStore((state) => state.auth.user)
  const session = useAuthStore((state) => state.auth.session)
  const token = useAuthStore((state) => state.auth.accessToken)
  const [now, setNow] = useState(Date.now)

  useEffect(() => {
    if (!session) return
    const remaining = session.expires_at * 1000 - Date.now()
    if (remaining <= 0) return
    const timer = window.setTimeout(
      () => setNow(Date.now()),
      Math.min(remaining, 2_147_483_647)
    )
    return () => window.clearTimeout(timer)
  }, [session, now])

  if (
    user?.role !== ROLE.SUPER_ADMIN ||
    user.status === 2 ||
    !token ||
    !session?.sid ||
    session.expires_at * 1000 <= Math.max(now, Date.now())
  ) {
    return null
  }
  return `${user.id}:${session.sid}`
}
