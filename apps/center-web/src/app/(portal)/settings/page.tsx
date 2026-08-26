"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import { Button, Card, Field, Input, PageHeader, Select } from "@/components/ui";

export default function SettingsPage() {
  const auth = useAuth(); const router = useRouter();
  const [name, setName] = useState(""); const [locale, setLocale] = useState("uz"); const [currentPassword, setCurrentPassword] = useState(""); const [newPassword, setNewPassword] = useState("");
  useEffect(() => { if (auth.profile) { setName(auth.profile.full_name || ""); setLocale(auth.profile.locale || "uz"); } }, [auth.profile]);
  return <><PageHeader title="Settings" subtitle="Center administrator profile and account security."/><div className="grid grid-2 section"><Card><h3>Profile</h3><div className="stack"><Field label="Full name"><Input value={name} onChange={(e) => setName(e.target.value)}/></Field><Field label="Locale"><Select value={locale} onChange={(e) => setLocale(e.target.value)}><option value="uz">Uzbek</option><option value="en">English</option><option value="ru">Russian</option></Select></Field><Button className="accent" disabled={!name.trim()} onClick={async () => { try { await api(portalPath("center","identity","/me"),auth.session!.access_token,json("PATCH",{full_name:name.trim(),locale})); await auth.refresh(); toast.success("Saved"); } catch (error:any) { toast.error(error.message); } }}>Save</Button></div></Card><Card><h3>Change password</h3><div className="stack"><Field label="Current password"><Input type="password" autoComplete="current-password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)}/></Field><Field label="New password"><Input type="password" autoComplete="new-password" minLength={10} value={newPassword} onChange={(e) => setNewPassword(e.target.value)}/></Field><Button className="primary" disabled={!currentPassword || newPassword.length < 10} onClick={async () => { try { await api(portalPath("center","identity","/me/password"),auth.session!.access_token,json("PATCH",{current_password:currentPassword,new_password:newPassword})); toast.success("Password changed. Please sign in again."); await auth.logout(); router.replace("/login"); } catch (error:any) { toast.error(error.message); } }}>Change password</Button><div className="muted" style={{fontSize:12}}>Changing the password revokes all active sessions for this account.</div></div></Card></div></>;
}
