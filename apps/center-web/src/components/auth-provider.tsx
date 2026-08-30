"use client";

import {createContext,useCallback,useContext,useEffect,useMemo,useRef,useState,type ReactNode} from "react";
import {ApiError,api,json,portalPath} from "@/lib/api";

export type Profile={user_id:string;organization_id:string|null;role:string;email:string;full_name:string;status:string;current_level:string|null;locale:string};
export type Session={access_token:string;refresh_token:string;token_type:string;expires_in:number;expires_at:string;profile?:Profile};
type Portal="admin"|"center"|"teacher"|"student";
type MFAChallenge={mfa_required:true;challenge_id:string;expires_in:number;profile?:Profile};
type Ctx={session:Session|null;profile:Profile|null;loading:boolean;error:string;mfaChallenge:string|null;login:(email:string,password:string)=>Promise<"authenticated"|"mfa_required">;verifyMfa:(code:string)=>Promise<void>;cancelMfa:()=>void;logout:()=>Promise<void>;refresh:()=>Promise<void>};
const C=createContext<Ctx|null>(null);

function isChallenge(value:Session|MFAChallenge): value is MFAChallenge{return "mfa_required" in value && value.mfa_required===true}

export function AuthProvider({portal,expectedRole,children}:{portal:Portal;expectedRole:string;children:ReactNode}){
  const [session,setSession]=useState<Session|null>(null);
  const [profile,setProfile]=useState<Profile|null>(null);
  const [loading,setLoading]=useState(true);
  const [error,setError]=useState("");
  const [mfaChallenge,setMfaChallenge]=useState<string|null>(null);
  const sessionRef=useRef<Session|null>(null);
  const storageKey=`ielts-auth-${portal}`;

  const save=useCallback((next:Session|null)=>{
    sessionRef.current=next; setSession(next);
    if(typeof window!=="undefined"){
      if(next)localStorage.setItem(storageKey,JSON.stringify(next));else localStorage.removeItem(storageKey);
    }
  },[storageKey]);

  const renew=useCallback(async(current:Session)=>{
    const next=await api<Session>(`/auth/${portal}/refresh`,"",json("POST",{refresh_token:current.refresh_token}));
    save(next);return next;
  },[portal,save]);

  const fetchProfile=useCallback(async(current:Session)=>{
    const p=await api<Profile>(portalPath(portal,"identity","/me"),current.access_token);
    if(p.role!==expectedRole)throw new ApiError(403,"role_forbidden","Bu account ushbu portal uchun ruxsatga ega emas.");
    setProfile(p);setError("");return p;
  },[expectedRole,portal]);

  const loadProfile=useCallback(async(current:Session,allowRenew=true)=>{
    try{await fetchProfile(current);return current}catch(e:any){
      if(allowRenew&&e instanceof ApiError&&e.code!=="role_forbidden"&&(e.status===401||e.status===403)){
        try{const next=await renew(current);await fetchProfile(next);return next}catch{}
      }
      save(null);setProfile(null);throw e;
    }
  },[fetchProfile,renew,save]);

  useEffect(()=>{let active=true;void(async()=>{try{const raw=localStorage.getItem(storageKey);if(!raw)return;const saved=JSON.parse(raw) as Session;if(!saved?.access_token||!saved?.refresh_token)return;save(saved);await loadProfile(saved)}catch(e:any){if(active){save(null);setProfile(null);setError(e?.message||"Sessiyani tiklab bo‘lmadi.")}}finally{if(active)setLoading(false)}})();return()=>{active=false}},[loadProfile,save,storageKey]);

  useEffect(()=>{if(!session)return;const exp=Date.parse(session.expires_at);if(!Number.isFinite(exp))return;const delay=Math.max(1000,exp-Date.now()-60000);const timer=window.setTimeout(()=>{void(async()=>{try{const next=await renew(sessionRef.current||session);await fetchProfile(next)}catch{save(null);setProfile(null)}})()},delay);return()=>window.clearTimeout(timer)},[session,renew,fetchProfile,save]);

  const login=useCallback(async(email:string,password:string)=>{
    setError("");setLoading(true);setMfaChallenge(null);
    try{
      const result=await api<Session|MFAChallenge>(`/auth/${portal}/login`,"",json("POST",{email,password}));
      if(isChallenge(result)){setMfaChallenge(result.challenge_id);return "mfa_required" as const}
      save(result);await fetchProfile(result);return "authenticated" as const;
    }catch(e:any){save(null);setProfile(null);setError(e?.message||"Kirish amalga oshmadi.");throw e}finally{setLoading(false)}
  },[portal,save,fetchProfile]);

  const verifyMfa=useCallback(async(code:string)=>{
    if(!mfaChallenge)throw new Error("MFA challenge topilmadi.");setLoading(true);setError("");
    try{const next=await api<Session>(`/auth/${portal}/mfa-verify`,"",json("POST",{challenge_id:mfaChallenge,code}));save(next);setMfaChallenge(null);await fetchProfile(next)}catch(e:any){setError(e?.message||"Tasdiqlash kodi noto‘g‘ri.");throw e}finally{setLoading(false)}
  },[mfaChallenge,portal,save,fetchProfile]);

  const cancelMfa=useCallback(()=>{setMfaChallenge(null);setError("")},[]);

  const logout=useCallback(async()=>{const current=sessionRef.current;try{if(current?.refresh_token)await api(`/auth/${portal}/logout`,"",json("POST",{refresh_token:current.refresh_token}))}catch{}finally{save(null);setProfile(null);setMfaChallenge(null);setError("")}},[portal,save]);

  const refresh=useCallback(async()=>{const current=sessionRef.current;if(!current)return;setLoading(true);try{const next=await renew(current);await fetchProfile(next)}finally{setLoading(false)}},[renew,fetchProfile]);

  const value=useMemo<Ctx>(()=>({session,profile,loading,error,mfaChallenge,login,verifyMfa,cancelMfa,logout,refresh}),[session,profile,loading,error,mfaChallenge,login,verifyMfa,cancelMfa,logout,refresh]);
  return <C.Provider value={value}>{children}</C.Provider>;
}
export function useAuth(){const v=useContext(C);if(!v)throw new Error("AuthProvider missing");return v}
