"use client";
import Link from "next/link";
import {usePathname,useRouter} from "next/navigation";
import {useEffect,type ReactNode} from "react";
import {BookPlus,BookOpenCheck,ClipboardCheck,House,Languages,ShieldCheck,Users,UsersRound} from "lucide-react";
import {useAuth} from "@/components/auth-provider";
import {Button,Pill} from "@/components/ui";
import {ThemeToggle} from "@/components/theme-provider";
const nav=[
  {href:"/",label:"Overview",icon:House},
  {href:"/students",label:"Students",icon:Users},
  {href:"/groups",label:"Groups",icon:UsersRound},
  {href:"/dictionary",label:"Dictionary",icon:Languages},
  {href:"/vocabulary-manager",label:"Add Vocabulary",icon:BookPlus},
  {href:"/homework",label:"Homework",icon:BookOpenCheck},
  {href:"/services",label:"Services",icon:ClipboardCheck},
  {href:"/security",label:"Security",icon:ShieldCheck},
];
export function PortalShell({children}:{children:ReactNode}){const pathname=usePathname();const router=useRouter();const auth=useAuth();useEffect(()=>{if(!auth.loading&&!auth.profile&&pathname!=="/login")router.replace("/login")},[auth.loading,auth.profile,pathname,router]);if(auth.loading)return <div className="login-wrap"><div className="muted">IELTS Teacher portal yuklanmoqda…</div></div>;if(!auth.profile)return null;return <div className="shell"><aside className="sidebar" aria-label="Primary navigation"><div className="brand"><span className="brandmark">IELTS</span><div>IELTS Teacher<div className="muted" style={{fontSize:11,fontWeight:500}}>Vocabulary & homework workspace</div></div></div><nav className="nav">{nav.map(item=>{const Icon=item.icon;const active=pathname===item.href;return <Link key={item.href} href={item.href} className={active?"active":""}><Icon size={17}/>{item.label}</Link>})}</nav><div className="sidebar-footer"><div style={{fontSize:13,fontWeight:650}}>{auth.profile.full_name}</div><div className="muted" style={{fontSize:11,margin:"4px 0 12px"}}>{auth.profile.email}</div><Button variant="outline" onClick={async()=>{await auth.logout();router.replace("/login")}}>Chiqish</Button></div></aside><main className="main"><header className="topbar"><b>Teacher workspace</b><div className="row"><ThemeToggle/><Pill>{auth.profile.role}</Pill></div></header><div className="content">{children}</div></main><nav className="mobile-nav" aria-label="Mobile navigation">{nav.map(item=><Link key={item.href} href={item.href}>{item.label}</Link>)}</nav></div>}
