/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Code2, Cpu, Sparkles } from 'lucide-react'

type Point = { x: number; y: number }

function Platform(props: { width: number; depth: number; height: number }) {
  const w = props.width / 2
  const d = props.depth
  const h = props.height
  return (
    <g className='omnivo-flow-platform' strokeLinejoin='round'>
      <path
        className='omnivo-flow-face-left'
        d={`M${-w} 0 0 ${d}v${h}l${-w} ${-d}Z`}
      />
      <path
        className='omnivo-flow-face-right'
        d={`M0 ${d} ${w} 0v${h}L0 ${d + h}Z`}
      />
      <path
        className='omnivo-flow-face-top'
        d={`M${-w} 0 0 ${-d} ${w} 0 0 ${d}Z`}
      />
    </g>
  )
}

function ModelNode(props: { point: Point; selected?: boolean }) {
  return (
    <g transform={`translate(${props.point.x} ${props.point.y})`}>
      <ellipse className='omnivo-flow-shadow' cx='0' cy='40' rx='76' ry='24' />
      <Platform width={154} depth={32} height={14} />
      <g
        transform='translate(0 -29)'
        className={props.selected ? 'omnivo-flow-selected' : 'omnivo-flow-idle'}
      >
        <Platform width={104} depth={23} height={42} />
        <Cpu
          x='-19'
          y='-18'
          width='38'
          height='38'
          strokeWidth='1.25'
          className='omnivo-flow-chip'
        />
        <path
          className='omnivo-flow-server-slots'
          d='M12 37 38 26M12 44 38 33M12 51 38 40'
        />
      </g>
      {props.selected && (
        <ellipse
          className='omnivo-flow-model-glow omnivo-flow-animated'
          cx='0'
          cy='3'
          rx='85'
          ry='39'
        />
      )}
    </g>
  )
}

function AppNode(props: { point: Point }) {
  return (
    <g transform={`translate(${props.point.x} ${props.point.y})`}>
      <ellipse className='omnivo-flow-shadow' cx='0' cy='45' rx='103' ry='28' />
      <Platform width={200} depth={43} height={16} />
      <g
        transform='matrix(1 .18 0 1 -76 -105)'
        className='omnivo-flow-app-window'
      >
        <rect width='152' height='112' rx='9' />
        <path className='omnivo-flow-window-divider' d='M0 23h152' />
        <g className='omnivo-flow-window-dots'>
          <circle cx='13' cy='12' r='2' />
          <circle cx='22' cy='12' r='2' />
          <circle cx='31' cy='12' r='2' />
        </g>
        <Code2 x='14' y='36' width='27' height='27' strokeWidth='1.5' />
        <path className='omnivo-flow-code-line' d='M54 44h58M54 55h38' />
        <path className='omnivo-flow-code-line' d='M17 80h117M17 92h85' />
        <path
          className='omnivo-flow-output omnivo-flow-animated'
          pathLength='100'
          d='M17 80h117M17 92h85'
        />
      </g>
    </g>
  )
}

function GatewayNode(props: { point: Point }) {
  return (
    <g transform={`translate(${props.point.x} ${props.point.y})`}>
      <ellipse className='omnivo-flow-shadow' cx='0' cy='53' rx='123' ry='34' />
      <Platform width={244} depth={53} height={18} />
      <g transform='translate(0 -49)' className='omnivo-flow-hub'>
        <Platform width={166} depth={37} height={67} />
        <path
          className='omnivo-flow-hub-seam'
          d='M-82 22 0 59 82 22M-82 43 0 80 82 43'
        />
        <image href='/omnivo-mark.svg' x='-39' y='-52' width='78' height='78' />
        <path className='omnivo-flow-hub-lights' d='m23 60 30-13m-30 23 17-8' />
      </g>
      <circle className='omnivo-flow-port' cx='0' cy='53' r='4' />
    </g>
  )
}

// Decorative geometry only: labels and the complete flow remain ordinary HTML.
// The compact composition rearranges the nodes instead of shrinking tiny text.
export function OmnivoFlowScene(props: { compact: boolean }) {
  const app = props.compact ? { x: 113, y: 320 } : { x: 177, y: 181 }
  const hub = props.compact ? { x: 310, y: 220 } : { x: 524, y: 181 }
  const model = props.compact ? { x: 514, y: 121 } : { x: 884, y: 181 }
  const inMidpoint = (app.x + 85 + hub.x - 88) / 2
  const outMidpoint = (hub.x + 88 + model.x - 60) / 2
  const incoming = `M${app.x + 85} ${app.y + 7}C${inMidpoint} ${app.y + 7} ${inMidpoint} ${hub.y + 7} ${hub.x - 88} ${hub.y + 7}`
  const outgoing = `M${hub.x + 88} ${hub.y + 7}C${outMidpoint} ${hub.y + 7} ${outMidpoint} ${model.y + 7} ${model.x - 60} ${model.y + 7}`
  const returning = `M${model.x} ${model.y + 42}C${model.x - 50} ${model.y + 158} ${hub.x + 65} ${hub.y + 110} ${hub.x} ${hub.y + 53}S${app.x + 60} ${app.y + 141} ${app.x} ${app.y + 43}`

  return (
    <svg
      className='omnivo-flow-scene'
      viewBox={props.compact ? '0 0 630 445' : '0 0 1120 350'}
      fill='none'
      aria-hidden='true'
      focusable='false'
    >
      <g className='omnivo-flow-floor'>
        <path
          d={
            props.compact
              ? 'M33 339 472 55 610 122 170 406Z'
              : 'M45 221 740 18 1072 171 373 340Z'
          }
        />
        {!props.compact && (
          <path d='m177 181 340 154M350 130l340 154M524 80l340 154M175 257 870 54M307 317l695-203' />
        )}
      </g>
      {!props.compact && (
        <g className='omnivo-flow-idle-routes'>
          <path d='M612 188C709 188 721 56 875 56' />
          <path d='M612 188C727 188 776 287 991 287' />
        </g>
      )}
      <g className='omnivo-flow-track' strokeLinecap='round'>
        <path d={incoming} />
        <path d={outgoing} />
        <path d={returning} />
      </g>
      <g className='omnivo-flow-route' strokeLinecap='round'>
        <path d={incoming} />
        <path d={outgoing} />
        <path className='omnivo-flow-return-route' d={returning} />
      </g>
      <g className='omnivo-flow-packets' strokeLinecap='round'>
        <path
          className='omnivo-flow-incoming omnivo-flow-animated'
          pathLength='100'
          d={incoming}
        />
        <path
          className='omnivo-flow-outgoing omnivo-flow-animated'
          pathLength='100'
          d={outgoing}
        />
        <path
          className='omnivo-flow-returning omnivo-flow-animated'
          pathLength='100'
          d={returning}
        />
      </g>
      {!props.compact && <ModelNode point={{ x: 923, y: 52 }} />}
      <AppNode point={app} />
      <GatewayNode point={hub} />
      <ModelNode point={model} selected />
      {!props.compact && <ModelNode point={{ x: 1036, y: 282 }} />}
      <g
        className='omnivo-flow-spark'
        transform={`translate(${hub.x + 78} ${hub.y - 131})`}
      >
        <Sparkles width='24' height='24' strokeWidth='1.2' />
      </g>
    </svg>
  )
}
