import client from './client'
export interface NotificationPayload { type:string; title:string; content:string; user_id?:number }
export const sendNotification=(payload:NotificationPayload)=>client.post('/admin/notifications',payload).then(r=>r.data)
export const broadcastNotification=(payload:NotificationPayload)=>client.post<{sent:number}>('/admin/notifications/broadcast',payload).then(r=>r.data)
