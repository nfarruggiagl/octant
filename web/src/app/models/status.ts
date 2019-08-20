export function operateChanges(
  list: Message,
  state?: string,
  errorInfo?: string,
  timeStamp?: 0
) {
  list.state = state;
  list.timeStamp = new Date().getTime();
  return list;
}

export const State = {
  error: 'error',
  warning: 'warning',
};

export interface Tab {
  displayName: string;
  streamName: string;
}

export interface Message {
  message: string;
  state: string;
  timeStamp: number;
  timeDiff: string;
}
