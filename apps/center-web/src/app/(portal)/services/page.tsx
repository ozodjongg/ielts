"use client";

import { useState } from "react";
import { BookOpenCheck, Sigma } from "lucide-react";
import EnglishServicesPanel from "@/components/services/english-services";
import SATServicesPanel from "@/components/services/sat-services";
import { Button, Card } from "@/components/ui";

type Tab = "english" | "sat";

export default function ServicesPage() {
  const [tab, setTab] = useState<Tab>("english");
  return <>
    <Card className="section">
      <div className="row" style={{ flexWrap: "wrap" }}>
        <Button variant={tab === "english" ? "default" : "outline"} onClick={() => setTab("english")}><BookOpenCheck size={16} />English</Button>
        <Button variant={tab === "sat" ? "default" : "outline"} onClick={() => setTab("sat")}><Sigma size={16} />SAT Math</Button>
      </div>
    </Card>
    <div className="section">{tab === "english" ? <EnglishServicesPanel /> : <SATServicesPanel />}</div>
  </>;
}
