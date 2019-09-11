import {AnimationTriggerMetadata, trigger, state, animate, transition, style } from '@angular/animations';

// This value should match the value used for the Status View styles in 'status.component.scss'
const statusViewRightOut = '-13.5rem';
const statusViewRightIn = 0;

export const SlideInOutAnimation: AnimationTriggerMetadata =
    trigger('SlideInOutAnimation', [
        state('in', style({
            right: statusViewRightIn
        })),
        state('out', style({
            right: statusViewRightOut
        })),
        transition('out => in', [
            animate('.5s ease', style({
                right: statusViewRightIn
            }))
        ]),
        transition('in => out', [
            animate('.5s ease', style({
                right: statusViewRightOut
            }))
        ])
    ]);
