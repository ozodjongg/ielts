"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { Palette } from "lucide-react";
import { Toaster } from "sonner";

export type Theme = "pearl"|"midnight"|"ocean"|"emerald"|"violet"|"rose"|"amber"|"sky"|"indigo"|"mint"|"slate"|"sunset";
const themes:{value:Theme;label:string;dark?:boolean;color:string}[]=[
  {value:"pearl",label:"Pearl",color:"#18181b"},{value:"midnight",label:"Midnight",dark:true,color:"#6366f1"},
  {value:"ocean",label:"Ocean",color:"#0284c7"},{value:"emerald",label:"Emerald",color:"#059669"},
  {value:"violet",label:"Violet",color:"#7c3aed"},{value:"rose",label:"Rose",color:"#e11d48"},
  {value:"amber",label:"Amber",color:"#d97706"},{value:"sky",label:"Sky",color:"#0ea5e9"},
  {value:"indigo",label:"Indigo",color:"#4f46e5"},{value:"mint",label:"Mint",color:"#0f766e"},
  {value:"slate",label:"Slate",dark:true,color:"#38bdf8"},{value:"sunset",label:"Sunset",color:"#ea580c"},
];
type ThemeContextValue={theme:Theme;setTheme:(theme:Theme)=>void;themes:typeof themes};
const ThemeContext=createContext<ThemeContextValue|null>(null);
function isTheme(v:string|null):v is Theme{return themes.some(t=>t.value===v)}
function applyTheme(theme:Theme){const item=themes.find(t=>t.value===theme)!;document.documentElement.dataset.theme=theme;document.documentElement.style.colorScheme=item.dark?"dark":"light";const meta=document.querySelector('meta[name="theme-color"]');if(meta)meta.setAttribute("content",item.color)}
export function ThemeProvider({children}:{children:ReactNode}){const[theme,setThemeState]=useState<Theme>("pearl");useEffect(()=>{const stored=window.localStorage.getItem("ielts-theme");const initial=isTheme(stored)?stored:"pearl";setThemeState(initial);applyTheme(initial)},[]);const setTheme=(next:Theme)=>{setThemeState(next);window.localStorage.setItem("ielts-theme",next);applyTheme(next)};return <ThemeContext.Provider value={{theme,setTheme,themes}}>{children}</ThemeContext.Provider>}
export function useTheme(){const value=useContext(ThemeContext);if(!value)throw new Error("useTheme must be used inside ThemeProvider");return value}
export function ThemeToggle(){const{theme,setTheme,themes}=useTheme();return <label className="theme-picker" title="Rangli tema"><Palette size={15}/><select aria-label="Rangli tema" value={theme} onChange={e=>setTheme(e.target.value as Theme)}>{themes.map(t=><option key={t.value} value={t.value}>{t.label}</option>)}</select></label>}
export const ThemePicker=ThemeToggle;
export function ThemeToaster(){const{theme}=useTheme();const dark=theme==="midnight"||theme==="slate";return <Toaster position="top-right" theme={dark?"dark":"light"} closeButton/>}
