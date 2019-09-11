import { Component, Input, Output, EventEmitter } from '@angular/core';
import { Message } from 'src/app/models/status';

@Component({
  selector: 'app-message-details',
  templateUrl: './message-details.component.html',
  styleUrls: ['./message-details.component.scss']
})
export class MessageDetailsComponent {
  @Input('message') message: Message | undefined;
  @Output() onCloseDetails: EventEmitter<boolean> = new EventEmitter();
}