"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Button, Card, Field, Input, PageHeader, Select } from "@/components/ui";
import { api, json, portalPath } from "@/lib/api";

export default function ProfilePage() {
  const auth = useAuth();
  const router = useRouter();
  const [name, setName] = useState("");
  const [locale, setLocale] = useState("uz");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setName(auth.profile?.full_name ?? "");
    setLocale(auth.profile?.locale ?? "uz");
  }, [auth.profile?.full_name, auth.profile?.locale]);

  const saveProfile = async () => {
    if (!auth.session) return;
    if (name.trim().length < 2) {
      toast.error("Full name is required.");
      return;
    }
    try {
      setSaving(true);
      await api(
        portalPath("student", "identity", "/me"),
        auth.session.access_token,
        json("PATCH", { full_name: name.trim(), locale }),
      );
      await auth.refresh();
      toast.success("Profile saved");
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Profile could not be saved");
    } finally {
      setSaving(false);
    }
  };

  const changePassword = async () => {
    if (!auth.session) return;
    if (!currentPassword || newPassword.length < 10) {
      toast.error("Enter the current password and a new password of at least 10 characters.");
      return;
    }
    try {
      setSaving(true);
      await api(
        portalPath("student", "identity", "/me/password"),
        auth.session.access_token,
        json("PATCH", { current_password: currentPassword, new_password: newPassword }),
      );
      toast.success("Password changed. Please sign in again.");
      await auth.logout();
      router.replace("/login");
    } catch (error: unknown) {
      toast.error(error instanceof Error ? error.message : "Password could not be changed");
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <PageHeader title="Profile" subtitle="Personal details and account security." />
      <div className="grid grid-2 section">
        <Card className="p-6">
          <h3 className="text-lg font-semibold">Profile</h3>
          <div className="stack section">
            <Field label="Full name"><Input value={name} onChange={(event) => setName(event.target.value)} autoComplete="name" /></Field>
            <Field label="Locale">
              <Select value={locale} onChange={(event) => setLocale(event.target.value)}>
                <option value="uz">Uzbek</option>
                <option value="en">English</option>
                <option value="ru">Russian</option>
              </Select>
            </Field>
            <Field label="Current level"><Input disabled value={auth.profile?.current_level ?? "A1"} /></Field>
            <Button disabled={saving} onClick={() => void saveProfile()}>Save profile</Button>
          </div>
        </Card>
        <Card className="p-6">
          <h3 className="text-lg font-semibold">Change password</h3>
          <div className="stack section">
            <Field label="Current password">
              <Input type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} />
            </Field>
            <Field label="New password" hint="Use at least 10 characters.">
              <Input type="password" autoComplete="new-password" minLength={10} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} />
            </Field>
            <Button disabled={saving} onClick={() => void changePassword()}>Change password</Button>
            <div className="muted text-xs">All existing sessions are revoked after a password change.</div>
          </div>
        </Card>
      </div>
    </>
  );
}
