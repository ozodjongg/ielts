import Link from "next/link";
import { Card } from "@/components/ui";

export default function NotFound() {
  return (
    <div className="login-wrap">
      <Card className="login-card">
        <div className="brand"><span className="brandmark">IELTS</span><span>IELTS Student</span></div>
        <h1 className="title">404</h1>
        <p className="subtitle">Soralgan sahifa topilmadi.</p>
        <Link className="section inline-flex h-9 items-center rounded-md border border-[var(--foreground)] bg-[var(--foreground)] px-4 text-sm font-medium text-[var(--background)]" href="/">Bosh sahifaga qaytish</Link>
      </Card>
    </div>
  );
}
