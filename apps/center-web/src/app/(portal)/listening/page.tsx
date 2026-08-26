"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { api, json, portalPath } from "@/lib/api";
import { useAuth } from "@/components/auth-provider";
import {
    Button,
    Card,
    Empty,
    Field,
    Input,
    PageHeader,
    Pill,
    Select,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
    TableWrap,
    Textarea,
} from "@/components/ui";

type Audio = {
    id: string;
    title: string;
    level?: string | null;
    size_bytes: number;
    max_plays: number;
    allow_seek: boolean;
    status: string;
};

type ListeningSet = {
    id: string;
    title: string;
    audio_id: string;
    level?: string | null;
};

type Assignment = {
    id: string;
    set_id: string;
    target_type: string;
    target_id?: string | null;
    due_at?: string | null;
    created_at: string;
};

type Group = {
    id: string;
    name: string;
};

type Student = {
    user_id: string;
    full_name: string;
};

type TargetType = "all" | "group" | "student";

type TargetOption = {
    id: string;
    label: string;
};

export default function ListeningPage() {
    const auth = useAuth();

    const [audio, setAudio] = useState<Audio[]>([]);
    const [sets, setSets] = useState<ListeningSet[]>([]);
    const [assignments, setAssignments] = useState<Assignment[]>([]);
    const [groups, setGroups] = useState<Group[]>([]);
    const [students, setStudents] = useState<Student[]>([]);

    // Audio upload
    const [file, setFile] = useState<File | null>(null);
    const [title, setTitle] = useState("");
    const [level, setLevel] = useState("B1");
    const [maxPlays, setMaxPlays] = useState(2);

    // Listening set
    const [listeningSetTitle, setListeningSetTitle] = useState("");
    const [audioID, setAudioID] = useState("");

    const [questions, setQuestions] = useState(
        '[{"id":"q1","prompt":"What is the main idea?","options":["A","B","C","D"],"base_points":2}]',
    );

    const [answers, setAnswers] = useState('{"q1":"A"}');

    // Assignment
    const [assignmentSet, setAssignmentSet] = useState("");
    const [targetType, setTargetType] = useState<TargetType>("all");
    const [targetID, setTargetID] = useState("");

    const [loading, setLoading] = useState(false);
    const [uploading, setUploading] = useState(false);
    const [creatingSet, setCreatingSet] = useState(false);
    const [creatingAssignment, setCreatingAssignment] = useState(false);

    const load = useCallback(async () => {
        if (!auth.session) return;

        setLoading(true);

        try {
            const [audioResponse, setsResponse, assignmentsResponse, groupsResponse, studentsResponse] =
                await Promise.all([
                    api<{ items: Audio[] }>(
                        portalPath("center", "listening", "/audio"),
                        auth.session.access_token,
                    ),

                    api<{ items: ListeningSet[] }>(
                        portalPath("center", "listening", "/sets"),
                        auth.session.access_token,
                    ),

                    api<{ items: Assignment[] }>(
                        portalPath("center", "listening", "/assignments"),
                        auth.session.access_token,
                    ),

                    api<{ items: Group[] }>(
                        portalPath("center", "tenant", "/groups"),
                        auth.session.access_token,
                    ),

                    api<{ items: Student[] }>(
                        portalPath("center", "tenant", "/students"),
                        auth.session.access_token,
                    ),
                ]);

            const audioItems = audioResponse.items ?? [];
            const setItems = setsResponse.items ?? [];

            setAudio(audioItems);
            setSets(setItems);
            setAssignments(assignmentsResponse.items ?? []);
            setGroups(groupsResponse.items ?? []);
            setStudents(studentsResponse.items ?? []);

            setAudioID((current) => {
                if (current && audioItems.some((item) => item.id === current)) {
                    return current;
                }

                return audioItems[0]?.id ?? "";
            });

            setAssignmentSet((current) => {
                if (current && setItems.some((item) => item.id === current)) {
                    return current;
                }

                return setItems[0]?.id ?? "";
            });
        } finally {
            setLoading(false);
        }
    }, [auth.session]);

    useEffect(() => {
        void load().catch((error: unknown) => {
            toast.error(
                error instanceof Error
                    ? error.message
                    : "Listening data could not be loaded",
            );
        });
    }, [load]);

    const targets = useMemo<TargetOption[]>(() => {
        if (targetType === "group") {
            return groups.map((group) => ({
                id: group.id,
                label: group.name,
            }));
        }

        if (targetType === "student") {
            return students.map((student) => ({
                id: student.user_id,
                label: student.full_name,
            }));
        }

        return [];
    }, [targetType, groups, students]);

    const setByID = useMemo(
        () => new Map(sets.map((item) => [item.id, item])),
        [sets],
    );

    async function upload() {
        if (!auth.session) {
            toast.error("Session is unavailable");
            return;
        }

        if (!file) {
            toast.error("Audio file is required");
            return;
        }

        const safeMaxPlays = Math.min(
            10,
            Math.max(1, Number.isFinite(maxPlays) ? maxPlays : 2),
        );

        const form = new FormData();

        form.set("audio", file);
        form.set("title", title.trim() || file.name);
        form.set("level", level);
        form.set("max_plays", String(safeMaxPlays));
        form.set("allow_seek", "false");

        setUploading(true);

        try {
            await api(
                portalPath("center", "listening", "/audio"),
                auth.session.access_token,
                {
                    method: "POST",
                    body: form,
                },
            );

            toast.success("Private audio uploaded");

            setFile(null);
            setTitle("");
            setMaxPlays(2);

            await load();
        } catch (error: unknown) {
            toast.error(
                error instanceof Error
                    ? error.message
                    : "Audio upload failed",
            );
        } finally {
            setUploading(false);
        }
    }

    async function createSet() {
        if (!auth.session) {
            toast.error("Session is unavailable");
            return;
        }

        if (!audioID) {
            toast.error("Audio is required");
            return;
        }

        if (!listeningSetTitle.trim()) {
            toast.error("Set title is required");
            return;
        }

        let parsedQuestions: unknown;
        let parsedAnswers: unknown;

        try {
            parsedQuestions = JSON.parse(questions);
            parsedAnswers = JSON.parse(answers);
        } catch {
            toast.error("Questions or answer-key JSON is invalid");
            return;
        }

        if (!Array.isArray(parsedQuestions)) {
            toast.error("Questions JSON must be an array");
            return;
        }

        if (
            typeof parsedAnswers !== "object" ||
            parsedAnswers === null ||
            Array.isArray(parsedAnswers)
        ) {
            toast.error("Answer key JSON must be an object");
            return;
        }

        setCreatingSet(true);

        try {
            await api(
                portalPath("center", "listening", "/sets"),
                auth.session.access_token,
                json("POST", {
                    audio_id: audioID,
                    title: listeningSetTitle.trim(),
                    level,
                    questions: parsedQuestions,
                    answer_key: parsedAnswers,
                }),
            );

            toast.success("Listening set created");

            setListeningSetTitle("");

            await load();
        } catch (error: unknown) {
            toast.error(
                error instanceof Error
                    ? error.message
                    : "Listening set could not be created",
            );
        } finally {
            setCreatingSet(false);
        }
    }

    async function createAssignment() {
        if (!auth.session) {
            toast.error("Session is unavailable");
            return;
        }

        if (!assignmentSet) {
            toast.error("Listening set is required");
            return;
        }

        if (targetType !== "all" && !targetID) {
            toast.error("Target item is required");
            return;
        }

        const body: Record<string, unknown> = {
            set_id: assignmentSet,
            target_type: targetType,
        };

        if (targetType !== "all") {
            body.target_id = targetID;
        }

        setCreatingAssignment(true);

        try {
            await api(
                portalPath("center", "listening", "/assignments"),
                auth.session.access_token,
                json("POST", body),
            );

            toast.success("Listening assignment created");

            await load();
        } catch (error: unknown) {
            toast.error(
                error instanceof Error
                    ? error.message
                    : "Listening assignment could not be created",
            );
        } finally {
            setCreatingAssignment(false);
        }
    }

    return (
        <>
            <PageHeader
                title="Listening"
                subtitle="Private audio, play limits, secure playback, sets and tenant-scoped assignments."
            />

            <div className="grid grid-2 section">
                <Card>
                    <h3>Upload private audio</h3>

                    <div className="stack">
                        <Field label="Title">
                            <Input
                                value={title}
                                onChange={(event) => setTitle(event.target.value)}
                                placeholder="Audio title"
                            />
                        </Field>

                        <Field label="Audio file">
                            <Input
                                type="file"
                                accept="audio/webm,audio/ogg,audio/mpeg,audio/mp4,audio/wav,audio/x-wav"
                                onChange={(event) => {
                                    setFile(event.target.files?.[0] ?? null);
                                }}
                            />
                        </Field>

                        <div className="grid grid-2">
                            <Field label="Level">
                                <Select
                                    value={level}
                                    onChange={(event) => setLevel(event.target.value)}
                                >
                                    {["A1", "A2", "B1", "B2", "C1", "C2"].map((value) => (
                                        <option key={value} value={value}>
                                            {value}
                                        </option>
                                    ))}
                                </Select>
                            </Field>

                            <Field label="Max plays">
                                <Input
                                    type="number"
                                    min={1}
                                    max={10}
                                    value={maxPlays}
                                    onChange={(event) => {
                                        const value = Number(event.target.value);

                                        setMaxPlays(
                                            Number.isFinite(value)
                                                ? Math.min(10, Math.max(1, value))
                                                : 1,
                                        );
                                    }}
                                />
                            </Field>
                        </div>

                        <Button
                            className="accent"
                            onClick={() => void upload()}
                            disabled={!file || uploading}
                        >
                            {uploading ? "Uploading..." : "Upload"}
                        </Button>
                    </div>
                </Card>

                <Card>
                    <h3>Create listening set</h3>

                    <div className="stack">
                        <Field label="Set title">
                            <Input
                                value={listeningSetTitle}
                                onChange={(event) =>
                                    setListeningSetTitle(event.target.value)
                                }
                                placeholder="Listening set title"
                            />
                        </Field>

                        <Field label="Audio">
                            <Select
                                value={audioID}
                                onChange={(event) => setAudioID(event.target.value)}
                            >
                                <option value="">Select audio…</option>

                                {audio.map((item) => (
                                    <option key={item.id} value={item.id}>
                                        {item.title}
                                    </option>
                                ))}
                            </Select>
                        </Field>

                        <Field label="Questions JSON">
                            <Textarea
                                value={questions}
                                onChange={(event) => setQuestions(event.target.value)}
                            />
                        </Field>

                        <Field label="Answer key JSON (server-only)">
                            <Textarea
                                value={answers}
                                onChange={(event) => setAnswers(event.target.value)}
                            />
                        </Field>

                        <Button
                            onClick={() => void createSet()}
                            disabled={
                                !audioID ||
                                !listeningSetTitle.trim() ||
                                creatingSet
                            }
                        >
                            {creatingSet ? "Creating..." : "Create set"}
                        </Button>
                    </div>
                </Card>
            </div>

            <Card className="section">
                <h3>Create listening assignment</h3>

                <div className="grid grid-3">
                    <Field label="Listening set">
                        <Select
                            value={assignmentSet}
                            onChange={(event) =>
                                setAssignmentSet(event.target.value)
                            }
                        >
                            <option value="">Select…</option>

                            {sets.map((item) => (
                                <option key={item.id} value={item.id}>
                                    {item.title}
                                </option>
                            ))}
                        </Select>
                    </Field>

                    <Field label="Target">
                        <Select
                            value={targetType}
                            onChange={(event) => {
                                setTargetType(event.target.value as TargetType);
                                setTargetID("");
                            }}
                        >
                            <option value="all">All students</option>
                            <option value="group">Group</option>
                            <option value="student">Student</option>
                        </Select>
                    </Field>

                    {targetType !== "all" && (
                        <Field label="Target item">
                            <Select
                                value={targetID}
                                onChange={(event) =>
                                    setTargetID(event.target.value)
                                }
                            >
                                <option value="">Select…</option>

                                {targets.map((item) => (
                                    <option key={item.id} value={item.id}>
                                        {item.label}
                                    </option>
                                ))}
                            </Select>
                        </Field>
                    )}
                </div>

                <Button
                    className="accent section"
                    onClick={() => void createAssignment()}
                    disabled={
                        !assignmentSet ||
                        (targetType !== "all" && !targetID) ||
                        creatingAssignment
                    }
                >
                    {creatingAssignment ? "Assigning..." : "Assign set"}
                </Button>
            </Card>

            <div className="section">
                <h3>Audio library</h3>

                {loading && audio.length === 0 ? (
                    <Empty>Loading audio...</Empty>
                ) : audio.length === 0 ? (
                    <Empty>No audio uploaded yet.</Empty>
                ) : (
                    <TableWrap>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>Audio</TableHead>
                                    <TableHead>Level</TableHead>
                                    <TableHead>Size</TableHead>
                                    <TableHead>Max plays</TableHead>
                                    <TableHead>Seek</TableHead>
                                    <TableHead>Status</TableHead>
                                </TableRow>
                            </TableHeader>

                            <TableBody>
                                {audio.map((item) => (
                                    <TableRow key={item.id}>
                                        <TableCell>
                                            <b>{item.title}</b>
                                        </TableCell>

                                        <TableCell>
                                            {item.level || "—"}
                                        </TableCell>

                                        <TableCell>
                                            {Math.round((item.size_bytes || 0) / 1024)} KB
                                        </TableCell>

                                        <TableCell>
                                            {item.max_plays}
                                        </TableCell>

                                        <TableCell>
                                            {item.allow_seek ? "Allowed" : "Blocked"}
                                        </TableCell>

                                        <TableCell>
                                            <Pill
                                                tone={
                                                    item.status === "active"
                                                        ? "ok"
                                                        : ""
                                                }
                                            >
                                                {item.status}
                                            </Pill>
                                        </TableCell>
                                    </TableRow>
                                ))}
                            </TableBody>
                        </Table>
                    </TableWrap>
                )}
            </div>

            <div className="section">
                <h3>Assignments</h3>

                {loading && assignments.length === 0 ? (
                    <Empty>Loading assignments...</Empty>
                ) : assignments.length === 0 ? (
                    <Empty>No listening assignments yet.</Empty>
                ) : (
                    <TableWrap>
                        <Table>
                            <TableHeader>
                                <TableRow>
                                    <TableHead>Set</TableHead>
                                    <TableHead>Target</TableHead>
                                    <TableHead>Due</TableHead>
                                    <TableHead>Created</TableHead>
                                </TableRow>
                            </TableHeader>

                            <TableBody>
                                {assignments.map((item) => (
                                    <TableRow key={item.id}>
                                        <TableCell>
                                            {setByID.get(item.set_id)?.title ||
                                                item.set_id.slice(0, 12)}
                                        </TableCell>

                                        <TableCell>
                                            {item.target_type}
                                        </TableCell>

                                        <TableCell>
                                            {item.due_at
                                                ? new Date(item.due_at).toLocaleDateString()
                                                : "—"}
                                        </TableCell>

                                        <TableCell>
                                            {new Date(
                                                item.created_at,
                                            ).toLocaleDateString()}
                                        </TableCell>
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