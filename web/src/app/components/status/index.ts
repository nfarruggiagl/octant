/**
 * Created by pengf on 5/11/2018.
 */

import { Type } from '@angular/core';
import { StatusComponent } from './status.component';

export * from './status.component';
export * from '../../models/status';
export * from './status.service';
export const STATUS_DIRECTIVES: Type<any>[] = [StatusComponent];
