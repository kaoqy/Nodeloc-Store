import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api/auth'
import type { User } from '../types'
export const useAuthStore=defineStore('auth',()=>{const token=ref(localStorage.getItem('admin_token')); const raw=localStorage.getItem('admin_user'); const user=ref<User|null>(raw?JSON.parse(raw):null); const isAuthenticated=computed(()=>!!token.value); async function login(identifier:string,password:string){const res=await api.login(identifier,password); token.value=res.tokens.access_token; user.value=res.user; localStorage.setItem('admin_token',token.value); localStorage.setItem('admin_user',JSON.stringify(user.value))} function logout(){token.value=null;user.value=null;localStorage.removeItem('admin_token');localStorage.removeItem('admin_user')} async function fetchUser(){const res=await api.me();user.value=res.user;localStorage.setItem('admin_user',JSON.stringify(user.value))} return{token,user,isAuthenticated,login,logout,fetchUser}})
