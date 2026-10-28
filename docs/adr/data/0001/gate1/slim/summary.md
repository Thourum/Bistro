# Gate 1 measurements

Startup (30 runs): p50/p95/p99 = 1628.42 / 1766.77 / 1776.61 ms.
Median bootstrap 95% CI: 1600.28–1668.56 ms (1000 resamples, seed 11622176).

| Fixture | Runs / frames | Pooled p50 / p95 / p99 (ms) | Run p95 min / median / max (ms) | Dropped, measured refresh ×1.5 | Dropped, 120 Hz ×1.5 |
|---|---|---|---|---|---|
| github:googleapis/google-cloud-go:20526 | 10 / 12000 | 16.70 / 18.70 / 37.90 | 18.60 / 18.60 / 18.70 | 1.75% | 96.66% |
| github:bench/large-diff:1 | 10 / 12000 | 16.60 / 35.50 / 38.30 | 34.80 / 35.50 / 36.20 | 10.56% | 81.38% |
| github:bench/large-diff:2 | 10 / 12000 | 16.70 / 18.50 / 18.70 | 17.60 / 18.10 / 18.60 | 0.01% | 99.92% |
| github:kubernetes/kubernetes:143358 | 10 / 12000 | 16.60 / 27.90 / 34.90 | 25.81 / 27.86 / 28.31 | 5.55% | 92.61% |

120 Hz budget = 8.33 ms; dropped threshold = 12.50 ms. Counts are late-frame shares, not an estimate of missed refresh slots.

| Memory phase | Runs | Per-run process sum min / median / max (MiB) |
|---|---|---|
| post-diff/generated | 5 | 746.76 / 754.29 / 771.52 |
| post-diff/synthetic-many-files | 5 | 757.01 / 763.82 / 767.63 |
| post-diff/synthetic-one-file | 5 | 726.35 / 731.13 / 746.16 |
| post-diff/vendoring | 5 | 703.05 / 714.12 / 721.38 |
| idle | 5 | 247.22 / 247.57 / 250.47 |

- Memory sums per-process footprint; shared allocations can be counted more than once.
- Measured-refresh dropped share uses each run’s idle-rAF median; 120 Hz is a separate fixed budget.
- Startup includes shell/app launch through PR-list ready and process exit, not just renderer t.
- Bootstrap CI is for the startup median, not p95/p99. Small n gives imprecise tails. See conditions.txt.
