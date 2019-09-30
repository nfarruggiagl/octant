import {AnimationTriggerMetadata, trigger, state, animate, transition, style } from '@angular/animations';

// This value should match the value used for the Status View styles in 'status.component.scss'
const statusViewBottomOut = '-10.5rem';
const statusViewBottomIn = 0;

export const SlideInOutAnimation: AnimationTriggerMetadata =
    trigger('SlideInOutAnimation', [
        state('in', style({
            bottom: statusViewBottomIn
        })),
        state('out', style({
            bottom: statusViewBottomOut
        })),
        transition('out => in', [
            animate('.5s ease', style({
                bottom: statusViewBottomIn
            }))
        ]),
        transition('in => out', [
            animate('.5s ease', style({
                bottom: statusViewBottomOut
            }))
        ])
    ]);
