// Every number that appears on a slide lives here, and nothing on a slide may
// come from anywhere else. All of these are MEASURED values recorded in
// backend/docs/checkpoints/phase-0..9.md — none are estimates, and none were
// rounded in the flattering direction.

module.exports = {
  team: { name: "PULL REQUEST", members: "Santhosh P · Hrithik Sankar R" },

  url: "crucible-backend-103350253775.us-central1.run.app",

  // Amplitude envelope of the real 27.29s recorded demo interview, extracted
  // from backend/internal/replay/fixtures/demo-ml-engineer.json (126 audio
  // events, PCM16 @24kHz) as 72 normalised RMS buckets. The near-zero bars are
  // genuine pauses in the recorded speech.
  waveform: [
    0.684, 0.362, 0.538, 0.758, 0.36, 0.389, 0.375, 0.405, 0.431, 0.299, 0.331,
    0.341, 0.042, 0.329, 0.43, 0.39, 0.296, 0.3, 0.348, 0.261, 0.266, 0.079,
    0.188, 0.712, 0.267, 0.51, 1.0, 0.586, 0.747, 0.581, 0.47, 0.731, 0.513,
    0.545, 0.475, 0.606, 0.393, 0.329, 0.006, 0.586, 0.515, 0.567, 0.508, 0.46,
    0.365, 0.339, 0.262, 0.043, 0.574, 0.966, 0.361, 0.679, 0.512, 0.457, 0.511,
    0.381, 0.401, 0.36, 0.439, 0.46, 0.256, 0.365, 0.012, 0.367, 0.354, 0.465,
    0.231, 0.315, 0.278, 0.249, 0.183, 0.052,
  ],

  latency: {
    turnBoundary: "966 ms",
    turnBoundaryRuns: [966, 1130, 1213, 1420],
    target: "1200 ms",
    floor: "892 ms",
    beforeFix: "1548–2309 ms",
    evaluation: "5.4 s",
    evaluationRange: "3.5–6.9 s",
    connect: "~2 s",
    digest: "15–17 s",
    roadmap: "30–60 s",
  },

  quality: {
    falseReds: "0",
    anchorDrop: "0%",
    links: "7/7",
    compliance: "8/8",
    leaks: "0",
    scores: { excellent: "9.60", vague: "3.55", fabricated: "1.60" },
  },

  robustness: {
    load: "10/10",
    chaos: "17/17",
    tests: "125",
    replayBytes: "1,310,142",
    replaySeconds: "27 s",
    replayTokens: "0",
  },

  code: { go: "11,912", goTest: "3,010", packages: "24", prompts: "11" },

  // Three warm runs each, same structured span-grading prompt (phase-0).
  modelAB: {
    cats: ["Run 1", "Run 2", "Run 3"],
    slow: { name: "gemini-3.5-flash", runs: [55.0, 7.0, 24.0] },
    fast: { name: "gemini-3.6-flash", runs: [4.6, 4.6, 4.1] },
  },

  cost: "total=378 · audio_in=127 · audio_out=163",
};
