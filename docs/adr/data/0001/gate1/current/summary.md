# Gate 1 measurements

Startup (30 runs): p50/p95/p99 = 1571.48 / 1654.26 / 1725.04 ms.
Median bootstrap 95% CI: 1552.53–1586.49 ms (1000 resamples, seed 11622176).

| Fixture | Runs / frames | Pooled p50 / p95 / p99 (ms) | Run p95 min / median / max (ms) | Dropped, measured refresh ×1.5 | Dropped, 120 Hz ×1.5 |
|---|---|---|---|---|---|
| github:googleapis/google-cloud-go:20526 | 10 / 12000 | 16.70 / 18.70 / 38.00 | 18.60 / 18.70 / 18.70 | 1.75% | 96.78% |
| github:bench/large-diff:1 | 10 / 12000 | 16.60 / 35.50 / 38.60 | 35.20 / 35.55 / 35.90 | 10.58% | 81.34% |
| github:bench/large-diff:2 | 10 / 12000 | 16.70 / 18.60 / 18.80 | 18.60 / 18.60 / 18.60 | 0.01% | 99.92% |
| github:kubernetes/kubernetes:143358 | 10 / 12000 | 16.60 / 27.60 / 34.90 | 26.01 / 27.56 / 28.40 | 5.55% | 92.64% |

120 Hz budget = 8.33 ms; dropped threshold = 12.50 ms. Counts are late-frame shares, not an estimate of missed refresh slots.

| Memory phase | Runs | Per-run process sum min / median / max (MiB) |
|---|---|---|
| post-diff/generated | 5 | 750.98 / 768.45 / 777.98 |
| post-diff/synthetic-many-files | 5 | 754.88 / 761.38 / 764.65 |
| post-diff/synthetic-one-file | 5 | 731.66 / 744.90 / 748.70 |
| post-diff/vendoring | 5 | 709.02 / 714.01 / 718.09 |
| idle | 5 | 246.57 / 247.69 / 249.57 |

- Memory sums per-process footprint; shared allocations can be counted more than once.
- Measured-refresh dropped share uses each run’s idle-rAF median; 120 Hz is a separate fixed budget.
- Startup includes shell/app launch through PR-list ready and process exit, not just renderer t.
- Bootstrap CI is for the startup median, not p95/p99. Small n gives imprecise tails. See conditions.txt.
