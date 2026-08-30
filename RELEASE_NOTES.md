# IELTS Platform — Production Final

## Major changes

- Product branding changed to **IELTS Platform** across all frontend portals and runtime identifiers.
- Architecture remains a single Go modular monolith instead of nine backend services.
- Added fourth `teacher` role and a dedicated Teacher Vercel portal.
- Canonical role set is now `admin`, `center`, `teacher`, `student`; migration converts legacy admin/center role values.
- Added first-party TOTP MFA with setup, verification, login challenge, AAL2 session upgrade, recovery codes and disable flow.
- Admin, center and teacher write operations require AAL2 by default.
- Added teacher-only vocabulary contributions.
- Added individual student extra-word assignments and automatic SRS enrollment.
- Added vocabulary homework with multi-student targeting, deadlines and completion tracking.
- Added Student Assigned workspace.
- Removed Center vocabulary mutation UI; center may manage users/groups but vocabulary creation is teacher-only.
- Added 12 shared visual themes across all four portals.
- Added responsive tablet/mobile layouts, touch targets, scroll-safe tables and mobile navigation.
- Added Railway teacher origin/MFA configuration and local Teacher port `3004`.
- Supabase and Termux are not part of the platform architecture.

## Deployment target

```text
Railway: Go backend + PostgreSQL + /data volume
Vercel:  Admin + Center + Teacher + Student
```
