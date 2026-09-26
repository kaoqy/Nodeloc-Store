import client from './client'
import type { SiteStatus } from '../types'

export async function siteStatus(): Promise<SiteStatus> {
  const { data } = await client.get<SiteStatus>('/system/status')
  return data
}
