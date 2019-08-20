import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable, Subject } from 'rxjs';
import { OperateInfo, StatusTab } from './status';
import getAPIBase from '../../services/common/getAPIBase';

@Injectable()
export class StatusService {
  subjects: Subject<any> = null;

  operationInfoSource = new Subject<OperateInfo>();
  operationInfo$ = this.operationInfoSource.asObservable();

  operationTab$ = new Observable<StatusTab[]>();

  constructor(private http: HttpClient) {}

  publishInfo(data: OperateInfo): void {
    this.operationInfoSource.next(data);
  }

  public getTabs() {
    const url = [getAPIBase(), 'api/v1/octant-status/tabs'].join('/');
    return this.http.get<StatusTab[]>(url);
  }
}
