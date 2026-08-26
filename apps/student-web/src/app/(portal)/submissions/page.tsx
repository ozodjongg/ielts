"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";

import { useAuth } from "@/components/auth-provider";
import { Empty, PageHeader, Pill, Table, TableBody, TableCell, TableHead, TableHeader, TableRow, TableWrap } from "@/components/ui";
import { api, portalPath } from "@/lib/api";

type Submission = {
  id: string;
  service_code: string;
  prompt_id: string;
  status: string;
  score?: number | null;
  submitted_at: string;
  review_notes?: string | null;
};

export default function ManualReviewHistoryPage() {
  const auth = useAuth();
  const [items, setItems] = useState<Submission[]>([]);

  useEffect(() => {
    if (!auth.session) return;
    let active = true;
    void api<{ items: Submission[] }>(portalPath("student", "review", "/submissions"), auth.session.access_token)
      .then((response) => {
        if (active) setItems(response.items ?? []);
      })
      .catch((error: unknown) => {
        if (active) toast.error(error instanceof Error ? error.message : "Review history could not be loaded");
      });
    return () => {
      active = false;
    };
  }, [auth.session]);

  return (
    <>
      <PageHeader
        title="Speaking & Writing reviews"
        subtitle="Responses are submitted from the assigned assessment itself. This page is your review history."
      />
      <div className="section">
        {items.length === 0 ? (
          <Empty>Open English Tests and start a Speaking, Writing or Mock assignment to create a manual submission.</Empty>
        ) : (
          <TableWrap>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Service</TableHead>
                  <TableHead>Prompt</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Score</TableHead>
                  <TableHead>Submitted</TableHead>
                  <TableHead>Notes</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>{item.service_code}</TableCell>
                    <TableCell className="mono">{item.prompt_id}</TableCell>
                    <TableCell><Pill>{item.status}</Pill></TableCell>
                    <TableCell>{item.score ?? "—"}</TableCell>
                    <TableCell>{new Date(item.submitted_at).toLocaleString()}</TableCell>
                    <TableCell>{item.review_notes || "—"}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableWrap>
        )}
      </div>
    </>
  );
}
