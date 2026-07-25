import { describe, expect, it } from 'vitest'

import { arcPath, axisAngles, normalise, polar, radarPoints, toPath } from './geometry'

const near = (value: number, expected: number, tolerance = 0.01) =>
  expect(Math.abs(value - expected)).toBeLessThan(tolerance)

describe('polar', () => {
  it('treats angle 0 as east', () => {
    const p = polar(100, 100, 50, 0)
    near(p.x, 150)
    near(p.y, 100)
  })

  it('treats -90 as north, because SVG y grows downward', () => {
    // Getting this backwards flips every radar upside down.
    const p = polar(100, 100, 50, -90)
    near(p.x, 100)
    near(p.y, 50)
  })

  it('sweeps clockwise on screen for increasing angles', () => {
    const p = polar(100, 100, 50, 90)
    near(p.y, 150)
  })
})

describe('axisAngles', () => {
  it('starts pointing up and spaces the rest evenly', () => {
    expect(axisAngles(4)).toEqual([-90, 0, 90, 180])
  })

  it('handles the three- and six-axis cases the product uses', () => {
    expect(axisAngles(3)).toEqual([-90, 30, 150])
    expect(axisAngles(6)).toEqual([-90, -30, 30, 90, 150, 210])
  })

  it('returns nothing for a degenerate count', () => {
    expect(axisAngles(0)).toEqual([])
  })
})

describe('radarPoints', () => {
  it('places a full-scale value on the outer ring', () => {
    const [top] = radarPoints([10], 100, 100, 60, 10)
    near(top.x, 100)
    near(top.y, 40)
  })

  it('collapses a zero to the centre', () => {
    const [origin] = radarPoints([0], 100, 100, 60, 10)
    near(origin.x, 100)
    near(origin.y, 100)
  })

  it('normalises against the SCALE, not against the largest value present', () => {
    // Normalising to the data would make every radar look full, and a weak
    // session indistinguishable from a strong one.
    const weak = radarPoints([2, 2, 2, 2], 100, 100, 60, 10)
    const strong = radarPoints([9, 9, 9, 9], 100, 100, 60, 10)
    expect(weak[0].y).toBeGreaterThan(strong[0].y) // further from the top edge
  })

  it('clamps out-of-range values rather than escaping the chart', () => {
    const [over] = radarPoints([99], 100, 100, 60, 10)
    near(over.y, 40)
    const [under] = radarPoints([-5], 100, 100, 60, 10)
    near(under.y, 100)
  })
})

describe('toPath', () => {
  it('builds an open path', () => {
    expect(toPath([{ x: 0, y: 0 }, { x: 10, y: 5 }])).toBe('M0.00,0.00 L10.00,5.00')
  })

  it('closes a polygon when asked', () => {
    expect(toPath([{ x: 0, y: 0 }, { x: 10, y: 5 }], true)).toMatch(/ Z$/)
  })

  it('returns empty for no points', () => {
    expect(toPath([])).toBe('')
  })
})

describe('arcPath', () => {
  it('sets the large-arc flag for the pace dial 220 degree sweep', () => {
    // Hardcoding this flag to 0 would draw the complementary arc — the 140
    // degree gap instead of the dial itself.
    const path = arcPath(100, 100, 60, 160, 380)
    expect(path).toMatch(/A60,60 0 1 1 /)
  })

  it('clears the flag for a sweep under 180 degrees', () => {
    expect(arcPath(100, 100, 60, 0, 90)).toMatch(/A60,60 0 0 1 /)
  })

  it('starts and ends where polar says it should', () => {
    const path = arcPath(100, 100, 60, 160, 380)
    const start = polar(100, 100, 60, 160)
    expect(path.startsWith(`M${start.x.toFixed(2)},${start.y.toFixed(2)}`)).toBe(true)
  })

  it('flips the sweep flag when running backwards', () => {
    expect(arcPath(100, 100, 60, 90, 0)).toMatch(/A60,60 0 0 0 /)
  })
})

describe('normalise', () => {
  it('maps a value onto 0..1', () => {
    expect(normalise(110, 60, 220)).toBeCloseTo(0.3125)
    expect(normalise(160, 60, 220)).toBeCloseTo(0.625)
  })

  it('clamps outside the range, so a fast talker still lands on the dial', () => {
    expect(normalise(400, 60, 220)).toBe(1)
    expect(normalise(10, 60, 220)).toBe(0)
  })

  it('does not divide by zero on a degenerate range', () => {
    expect(normalise(5, 5, 5)).toBe(0)
  })
})
