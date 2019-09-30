import { Component } from '@angular/core';
import { BehaviorSubject } from 'rxjs';
import { SlideInOutAnimation } from '../_animations/slide-in-out.animation';
import { Tab, Message } from '../../models/status';

const emptyMessages = { message: '', state: '', timeDiff: '', timeStamp: 0 };

@Component({
  selector: 'app-status-model',
  templateUrl: './status.component.html',
  styleUrls: ['./status.component.scss'],
  animations: [SlideInOutAnimation],
})
export class StatusComponent {
  tabs: Tab[] = [{
    displayName: 'Tab 1',
    streamName: 'Tab 1'
  }, {
    displayName: 'Tab 2',
    streamName: 'Tab 2'
  }, {
    displayName: 'Tab 3',
    streamName: 'Tab 3'
  }, {
    displayName: 'Tab 4',
    streamName: 'Tab 4'
  }];
  behavior = new BehaviorSubject<Message>(emptyMessages);
  messageList: any[] = [];
  animationState = 'out';

  private handleEvent = (message: MessageEvent) => {
    const data = JSON.parse(message.data);
    this.behavior.next(data);
    this.messageList.unshift(data);
  };

  toggleTitle(errorSpan) {
    errorSpan.style.display =
      errorSpan.style.display === 'block' ? 'none' : 'block';
  }

  slide() {
    this.animationState = this.animationState === 'out' ? 'in' : 'out';
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
