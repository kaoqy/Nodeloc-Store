import client from './client'
import type { SettingsMap } from '../types'
export const getSettings=()=>client.get<{settings:SettingsMap}>('/admin/settings').then(r=>r.data)
export const saveSettings=(settings:SettingsMap)=>client.post<{ok:boolean}>('/admin/settings',{settings}).then(r=>r.data)
export const testOAuth=()=>client.post<{ok:boolean,authorize_url:string}>('/admin/settings/oauth-test').then(r=>r.data)
export const testPayment=()=>client.post<{ok:boolean,msg:string}>('/admin/settings/payment-test').then(r=>r.data)
