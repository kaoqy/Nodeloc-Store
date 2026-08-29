import client from './client'
import type { PaginationParams, User } from '../types'
export const listUsers=(params?:PaginationParams)=>client.get<{data:User[]}>('/admin/users',{params}).then(r=>r.data)
export const getUser=(id:number)=>client.get<{user:User}>(`/admin/users/${id}`).then(r=>r.data)
export const toggleAdmin=(id:number)=>client.post<{user:User}>(`/admin/users/${id}/toggle-admin`).then(r=>r.data)
export const toggleActive=(id:number)=>client.post<{user:User}>(`/admin/users/${id}/toggle-active`).then(r=>r.data)
export const setRole=(id:number,role:string)=>client.post<{user:User}>(`/admin/users/${id}/role`,{role}).then(r=>r.data)
export const adjustPoints=(id:number,delta:number,reason:string)=>client.post<{user:User}>(`/admin/users/${id}/points`,{delta,reason}).then(r=>r.data)
