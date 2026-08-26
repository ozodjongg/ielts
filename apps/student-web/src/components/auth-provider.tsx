"use client";

import {createContext,useCallback,useContext,useEffect,useMemo,useRef,useState,type ReactNode} from "react";
import {ApiError,api,json,portalPath} from "@/lib/api";

export type Profile={user_id:string;organization_id:string|null;role:string;email:string;full_name:string;status:string;current_level:string|null;locale:string};
export type Session={access_token:string;refresh_token:string;token_type:string;expires_in:number;expires_at:string;profile?:Profile};
type Portal="admin"|"center"|"student";
type Ctx={session:Session|null;profile:Profile|null;loading:boolean;error:string;login:(email:string,password:string)=>Promise<void>;logout:()=>Promise<void>;refresh:()=>Promise<void>};
const C=createContext<Ctx|null>(null);

export function AuthProvider({portal,expectedRole,children}:{portal:Portal;expectedRole:string;children:ReactNode}){
  const [session,setSession]=useState<Session|null>(null);
  const [profile,setProfile]=useState<Profile|null>(null);
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState("");
  const sessionRef=useRef<Session|null>(null);
  const storageKey=`assessment-v5-auth-${portal}`;

  const save=useCallback((next:Session|null)=>{
    sessionRef.current=next;
    setSession(next);
    if(typeof window!=="undefined"){
      if(next)localStorage.setItem(storageKey,JSON.stringify(next));
      else localStorage.removeItem(storageKey);
    }
  },[storageKey]);

  const renew=useCallback(async(current:Session)=>{
    const next=await api<Session>(`/auth/${portal}/refresh`,"",json("POST",{refresh_token:current.refresh_token}));
    save(next);
    return next;
  },[portal,save]);

  const fetchProfile=useCallback(async(current:Session)=>{
    const p=await api<Profile>(portalPath(portal,"identity","/me"),current.access_token);
    if(p.role!==expectedRole)throw new ApiError(403,"role_forbidden","Bu account ushbu portal uchun ruxsatga ega emas.");
    setProfile(p);
    setError("");
    return p;
  },[expectedRole,portal]);

  const loadProfile=useCallback(async(current:Session,allowRenew=true)=>{
    try{
      await fetchProfile(current);
      return current;
    }catch(e:any){
      if(allowRenew && e instanceof ApiError && e.code!=="role_forbidden" && (e.status===401||e.status===403)){
        try{
          const next=await renew(current);
          await fetchProfile(next);
          return next;
        }catch{}
      }
      save(null);
      setProfile(null);
      throw e;
    }
  },[fetchProfile,renew,save]);

  useEffect(()=>{
    let active=true;
    void (async()=>{
      try{
        const raw=localStorage.getItem(storageKey);
        if(!raw)return;
        const saved=JSON.parse(raw) as Session;
        if(!saved?.access_token||!saved?.refresh_token)return;
        save(saved);
        await loadProfile(saved);
      }catch(e:any){
        if(active){
          save(null);
          setProfile(null);
          setError(e?.message||"Sessiyani tiklab bo‘lmadi.");
        }
      }finally{
        if(active)setLoading(false);
      }
    })();
    return()=>{active=false};
  },[loadProfile,save,storageKey]);

  useEffect(()=>{
    if(!session)return;
    const exp=Date.parse(session.expires_at);
    if(!Number.isFinite(exp))return;
    const delay=Math.max(1000,exp-Date.now()-60000);
    const timer=window.setTimeout(()=>{
      void (async()=>{
        try{
          const next=await renew(sessionRef.current||session);
          await fetchProfile(next);
        }catch{
          save(null);
          setProfile(null);
        }
      })();
    },delay);
    return()=>window.clearTimeout(timer);
  },[session,renew,fetchProfile,save]);

  const login=useCallback(async(email:string,password:string)=>{
    setError("");
    setLoading(true);
    try{
      const next=await api<Session>(`/auth/${portal}/login`,"",json("POST",{email,password}));
      save(next);
      await fetchProfile(next);
    }catch(e:any){
      save(null);
      setProfile(null);
      setError(e?.message||"Kirish amalga oshmadi.");
      throw e;
    }finally{
      setLoading(false);
    }
  },[portal,save,fetchProfile]);

  const logout=useCallback(async()=>{
    const current=sessionRef.current;
    try{
      if(current?.refresh_token)await api(`/auth/${portal}/logout`,"",json("POST",{refresh_token:current.refresh_token}));
    }catch{}finally{
      save(null);
      setProfile(null);
      setError("");
    }
  },[portal,save]);

  const refresh=useCallback(async()=>{
    const current=sessionRef.current;
    if(!current)return;
    setLoading(true);
    try{await loadProfile(current,true)}finally{setLoading(false)}
  },[loadProfile]);

  const value=useMemo<Ctx>(()=>({session,profile,loading,error,login,logout,refresh}),[session,profile,loading,error,login,logout,refresh]);
  return <C.Provider value={value}>{children}</C.Provider>;
}

export function useAuth(){const v=useContext(C);if(!v)throw new Error("AuthProvider missing");return v}
