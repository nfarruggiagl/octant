import { Component } from '@angular/core';
import { StatusService } from './status.service';
import { BehaviorSubject } from 'rxjs';
import { SlideInOutAnimation } from '../_animations/slide-in-out.animation';
import {
  ContentStreamService,
  Streamer,
} from 'src/app/services/content-stream/content-stream.service';
import { Tab, Message } from '../../models/status';

const emptyMessages = { message: '', state: '', timeDiff: '', timeStamp: 0 };

@Component({
  selector: 'app-status-model',
  templateUrl: './status.component.html',
  styleUrls: ['./status.component.scss'],
  animations: [SlideInOutAnimation],
})
export class StatusComponent {
  tabs: Tab[] = [];
  behavior = new BehaviorSubject<Message>(emptyMessages);
  messageList: Message[] = [];
  animationState = 'out';
  activeMessage: Message | undefined;
  activeMessageId: string | undefined;

  constructor(
    private statusService: StatusService,
    private contentStreamService: ContentStreamService
  ) {
    const streamer: Streamer = {
      behavior: this.behavior,
      handler: this.handleEvent,
    };

    // TODO: There needs to be a factory that sets up the streamers
    // for each of the different status types, as of right now, there is only "error"
    // so this is fine.
    this.statusService.getTabs().subscribe((data: Tab[]) => {
      this.tabs = data;
      this.tabs.forEach(tab => {
        this.contentStreamService.registerStreamer(tab.streamName, streamer);
      });
    });
  }

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
    if (this.animationState === 'out') {
      this.closeDetails();
    }
  }

  viewMessageDetails(messageId: string, message: Message) {
    this.activeMessageId = messageId;
    this.activeMessage = message;
  }

  closeDetails() {
    this.activeMessage = undefined;
    this.activeMessageId = undefined;
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