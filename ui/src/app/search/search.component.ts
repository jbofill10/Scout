import { CommonModule } from '@angular/common';
import { Component, Output, EventEmitter } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { InputTextModule } from 'primeng/inputtext';
import { SearchResultComponent } from './search-result/search-result.component';
import { HttpClient, HttpTransferCacheOptions } from '@angular/common/http';
import { SearchResult } from '../models/search-result.model';
import { Observable } from 'rxjs';

@Component({
  selector: 'app-search',
  standalone: true,
  imports: [FormsModule, InputTextModule, CommonModule, SearchResultComponent],
  templateUrl: './search.component.html',
  styleUrl: './search.component.scss'
})
export class SearchComponent {
  searchTerm: string = '';
  formSubmitted: boolean = false
  searchResults: SearchResult[] = [];

  @Output() search: EventEmitter<string> = new EventEmitter<string>();

  constructor(
    private http: HttpClient
  ) { }

  ngOnInit(): void {
    this.onSearch()
  }

  onSearch(): void {
    this.formSubmitted = true;
    var params = { "query": "naruto", "media_type": "series" }
    const result: Observable<SearchResult[]> = this.http.get<SearchResult[]>("http://localhost:22920/search", { params });


    result.subscribe({
      next: (data: SearchResult[]) => this.searchResults = data,
      error: (e) => console.log(e),
    });
  }
}
