# prompt.ai Implementation

All four requirements in `prompt.ai` are implemented.

## 1. Printer-ready Word placement test

- Added `data/placement/placement-test-paper.docx`.
- Added `data/placement/paper-v1.json` so the printed test and center answer-entry screen use the same question order.
- Added center-only download endpoint `GET /api/center/assessment/pre-registration/placement-paper`.
- Paper candidates can be assessed without a phone; center staff enter A/B/C/D answers after the paper test.

## 2. Placement before student registration

- Added `assessment.pre_registration_placements` migration.
- Added center page `/placement`.
- Candidate starts with name/contact only; no level is requested.
- Digital and paper modes both calculate a placement result before account creation.
- Student account creation uses the measured level and the backend verifies the created account level matches the placement result.
- Student remains a separate login-only portal.
- The Students page now sends new registrations to Placement instead of asking for an initial level.

## 3. Investor / center presentation

- Added `presentation/presentation.pptx`.
- The 12-slide deck explains the problem, architecture, placement flow, center/teacher/student workflows, learning services, security, deployment and value proposition.

## 4. Human-friendly Reviews page

- Replaced raw rubric JSON editing on `/reviews`.
- Speaking and writing each show four understandable criteria with 0–25 numeric inputs.
- Total score is calculated automatically and reviewer notes remain simple free text.

## Verification

`python tools/verify_prompt_ai.py` validates both the existing project contracts and these new prompt.ai requirements.
