import { NgModule } from '@angular/core';
import { BrowserModule } from '@angular/platform-browser';

import { AppRoutingModule } from './app-routing.module';
import { AppComponent } from './app.component';
import { Toolbar } from './toolbar/toolbar.component';
import { SearchComponent } from './search/search.component';
import { ScheduleComponent } from './schedule/schedule.component';
import { DashboardComponent } from './dashboard/dashboard.component';
import { HttpClientModule, provideHttpClient, withInterceptorsFromDi } from '@angular/common/http';
import { SearchResultComponent } from './search/search-result/search-result.component';
import { ImageModalComponent } from './search/image-modal/image-modal.component';

@NgModule({
    declarations: [
        AppComponent,
    ],
    imports: [
        BrowserModule,
        AppRoutingModule,
        SearchComponent,
        SearchResultComponent,
        ScheduleComponent,
        DashboardComponent,
        Toolbar
    ],
    providers: [provideHttpClient(withInterceptorsFromDi()), // New way to provide HttpClient
    ],
    bootstrap: [AppComponent]
})
export class AppModule { }