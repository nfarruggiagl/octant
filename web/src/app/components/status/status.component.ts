import { Component, OnInit, OnDestroy, HostListener } from '@angular/core';
import { StatusService } from './status.service';
import { Subscription, Observable, BehaviorSubject } from 'rxjs';
import { OperateInfo, OperationState } from './status';
import { SlideInOutAnimation } from '../_animations/slide-in-out.animation';
import {
  ContentStreamService,
  Streamer,
} from 'src/app/services/content-stream/content-stream.service';
import { StatusTab } from './status';

const emptyOperateInfo: OperateInfo = new OperateInfo();

@Component({
  selector: 'app-status-model',
  templateUrl: './status.component.html',
  styleUrls: ['./status.component.css'],
  animations: [SlideInOutAnimation],
})
export class StatusComponent implements OnInit, OnDestroy {
  tabs: StatusTab[] = [];
  behavior = new BehaviorSubject<OperateInfo>(emptyOperateInfo);
  batchInfoSubscription: Subscription;
  resultLists: OperateInfo[] = [];
  animationState = 'out';

  @HostListener('window:beforeunload', ['$event'])
  beforeUnloadHander(event) {
    // storage to localStorage
    const timp = new Date().getTime();
    localStorage.setItem(
      'operaion',
      JSON.stringify({ timp, data: this.resultLists })
    );
  }

  constructor(
    private statusService: StatusService,
    private contentStreamService: ContentStreamService
  ) {
    const streamer: Streamer = {
      behavior: this.behavior,
      handler: this.handleEvent,
    };

    this.contentStreamService.registerStreamer('statusError', streamer);
    this.batchInfoSubscription = statusService.operationInfo$.subscribe(
      data => {
        // this.resultLists = data;
        this.openSlide();
        if (data) {
          if (this.resultLists.length >= 50) {
            this.resultLists.splice(49, this.resultLists.length - 49);
          }
          this.resultLists.unshift(data);
        }
      }
    );
    this.statusService.getTabs().subscribe((data: StatusTab[]) => {
      this.tabs = data;
    });
  }

  private handleEvent = (message: MessageEvent) => {
    const data = JSON.parse(message.data);
    this.behavior.next(data);
  };

  public get runningLists(): OperateInfo[] {
    const runningList: OperateInfo[] = [];
    this.resultLists.forEach(data => {
      if (data.state === 'progressing') {
        runningList.push(data);
      }
    });
    return runningList;
  }

  public get failLists(): OperateInfo[] {
    const failedList: OperateInfo[] = [];
    this.resultLists.forEach(data => {
      if (data.state === 'failure') {
        failedList.push(data);
      }
    });
    return failedList;
  }

  ngOnInit() {
    const requestCookie = localStorage.getItem('operaion');
    if (requestCookie) {
      const operInfors: any = JSON.parse(requestCookie);
      if (operInfors) {
        if (new Date().getTime() - operInfors.timp > 1000 * 60 * 60 * 24) {
          localStorage.removeItem('operaion');
        } else {
          if (operInfors.data) {
            operInfors.data.forEach(operInfo => {
              if (operInfo.state === OperationState.progressing) {
                operInfo.state = OperationState.interrupt;
                operInfo.data.errorInf = 'operation been interrupted';
              }
            });
            this.resultLists = operInfors.data;
          }
        }
      }
    }
  }
  ngOnDestroy(): void {
    if (this.batchInfoSubscription) {
      this.batchInfoSubscription.unsubscribe();
    }
  }

  toggleTitle(errorSpan: any) {
    errorSpan.style.display =
      errorSpan.style.display === 'block' ? 'none' : 'block';
  }

  slideOut(): void {
    this.animationState = this.animationState === 'out' ? 'in' : 'out';
  }

  openSlide(): void {
    this.animationState = 'in';
  }

  TabEvent(): void {
    let timp: any;
    this.resultLists.forEach(data => {
      timp = new Date().getTime() - +data.timeStamp;
      data.timeDiff = this.calculateTime(timp);
    });
  }

  calculateTime(timp: number) {
    const dist = Math.floor(timp / 1000 / 60); // change to minute;
    if (dist > 0 && dist < 60) {
      return Math.floor(dist) + ' minute(s) ago';
    } else if (dist >= 60 && Math.floor(dist / 60) < 24) {
      return Math.floor(dist / 60) + ' hour(s) ago';
    } else if (Math.floor(dist / 60) >= 24) {
      return Math.floor(dist / 60 / 24) + ' day ago';
    } else {
      return 'less 1 minute';
    }
  }
}
