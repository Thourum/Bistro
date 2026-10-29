import { expect, it } from 'vitest';
import { summarizeFrames } from './stats';

it('summarizes frame percentiles without mutating deltas and counts only deltas above the dropped-frame threshold', () => {
  const deltas = [40, 10, 30, 20, 100];
  expect(summarizeFrames(deltas, 20)).toEqual({
    p50: 30,
    p95: 100,
    p99: 100,
    droppedFrameShare: 2 / 5,
  });
  expect(deltas).toEqual([40, 10, 30, 20, 100]);
  expect(summarizeFrames([], 20)).toEqual({
    p50: 0,
    p95: 0,
    p99: 0,
    droppedFrameShare: 0,
  });
});
