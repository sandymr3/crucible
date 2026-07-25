import s from './ThermalField.module.css'

/**
 * The ambient thermal field. Mounted once at the app root; every screen sits on
 * top of it.
 *
 * The signature (design PRD §1.1): the room gets hotter as you get pushed
 * harder. At band 2 the atmosphere is a cool aquamarine bloom at the edges of a
 * near-black room; at band 5 the blooms have shifted to ember, the vignette has
 * tightened, and the room feels close.
 *
 * The mechanism is entirely CSS. This component owns no state and re-renders
 * never: the hue comes from --heat-cold / --heat-hot / --heat-alpha, which are
 * set by the [data-band] attribute on <html>. To move the room, call setBand()
 * from lib/band — that is the whole implementation.
 *
 * Purely decorative, so it is hidden from assistive technology.
 */
export function ThermalField() {
  return (
    <div className={s.field} aria-hidden="true">
      <div className={s.grain} />
      <div className={s.vignette} />
    </div>
  )
}
